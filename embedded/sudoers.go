package embedded

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

const sudoersPath = "/etc/sudoers.d/colima"
const sudoersEmbeddedPath = "network/sudo.txt"

// SudoersInstaller provides the ability to run commands on the host
// for installing the sudoers file.
type SudoersInstaller interface {
	RunInteractive(args ...string) error
	RunWith(stdin io.Reader, stdout io.Writer, args ...string) error
}

// SudoersInstalled checks whether the current user is already granted every
// command colima's vmnet/incus dependencies need.
//
// The authoritative check asks sudo itself, via `sudo -n -l` -- the same
// non-interactive introspection a human uses to audit their own grants --
// rather than reading sudoersPath directly. Reading the file requires the
// invoking (possibly unprivileged, non-interactive) user to have read
// access to a file that is conventionally 0440 root:wheel; a user can be
// fully and correctly granted (e.g. via group membership on the grantee
// clause) while still being unable to read the policy file that grants it,
// which made the old file-read check produce false negatives and re-trigger
// InstallSudoers' interactive `sudo` prompt on every run for such users. See
// https://github.com/abiosoft/colima/issues (this patch) for the report.
//
// `sudo -n -l` may itself be inconclusive (no cached credentials at all, no
// grants of any kind, sudo missing from PATH); in that case we fall back to
// the legacy direct file-content comparison so existing installs and CI
// environments are unaffected.
func SudoersInstalled() bool {
	txt, err := ReadString(sudoersEmbeddedPath)
	if err != nil {
		return false
	}

	if ok, checked := sudoListSatisfiesGrants(txt); checked {
		return ok
	}

	return legacySudoersFileContains(txt)
}

// sudoListSatisfiesGrants derives the set of commands colima's dependencies
// require directly from the embedded sudoers template (single source of
// truth -- no separately-maintained command list to drift out of sync) and
// checks each is present in `sudo -n -l`'s output for the invoking user.
//
// checked reports whether `sudo -n -l` produced a usable answer; when false,
// the caller should fall back to the legacy file-content check.
func sudoListSatisfiesGrants(template string) (ok bool, checked bool) {
	out, err := exec.Command("sudo", "-n", "-l").Output()
	if err != nil {
		return false, false
	}

	list := string(out)
	for _, grant := range requiredGrants(template) {
		if !strings.Contains(list, grant) {
			return false, true
		}
	}
	return true, true
}

// requiredGrants extracts the literal command text (path + flags, including
// any trailing sudoers wildcard) from each non-comment grant line of the
// embedded sudoers template. `sudo -n -l` reproduces this same command text
// verbatim for a matching grant, regardless of how it reorders the
// preceding NOPASSWD/NOSETENV tags -- so a plain substring match is
// sufficient and avoids depending on tag order/formatting.
func requiredGrants(template string) []string {
	var grants []string
	for _, line := range strings.Split(template, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields {
			if strings.HasPrefix(f, "/") {
				grants = append(grants, strings.Join(fields[i:], " "))
				break
			}
		}
	}
	return grants
}

// legacySudoersFileContains is the original check: does sudoersPath's
// content contain the embedded template verbatim. Kept as a fallback for
// when `sudo -n -l` can't be evaluated.
func legacySudoersFileContains(txt string) bool {
	b, err := os.ReadFile(sudoersPath)
	if err != nil {
		return false
	}
	return bytes.Contains(b, []byte(txt))
}

// InstallSudoers installs the embedded sudoers file if it is not already
// installed with the expected content. This may prompt for a sudo password.
func InstallSudoers(host SudoersInstaller) error {
	if SudoersInstalled() {
		return nil
	}

	txt, err := ReadString(sudoersEmbeddedPath)
	if err != nil {
		return fmt.Errorf("error reading embedded sudoers file: %w", err)
	}

	log.Println("setting up network permissions, sudo password may be required")

	dir := filepath.Dir(sudoersPath)
	if err := host.RunInteractive("sudo", "mkdir", "-p", dir); err != nil {
		return fmt.Errorf("error preparing sudoers directory: %w", err)
	}

	stdin := strings.NewReader(txt)
	stdout := &bytes.Buffer{}
	if err := host.RunWith(stdin, stdout, "sudo", "sh", "-c", "cat > "+sudoersPath); err != nil {
		return fmt.Errorf("error writing sudoers file: %w", err)
	}

	return nil
}
