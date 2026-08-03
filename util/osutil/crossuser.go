package osutil

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// EnvNoCrossUser opts out of the cross-user fallbacks below (fork addition,
// see macports-ports-local#162), even when the target is owned by a user
// other than the caller.
const EnvNoCrossUser = "COLIMA_NO_CROSS_USER"

// CrossUserOwner reports the username owning path when it differs from the
// caller's effective uid, so callers can decide whether to re-exec via sudo.
//
// ok is false when: the caller opted out via EnvNoCrossUser; path does not
// exist (nothing to own cross-user, e.g. before the first `colima start`);
// the owning uid cannot be resolved to a username; or path is already owned
// by the caller — the common case, where no re-exec is needed.
func CrossUserOwner(path string) (owner string, ok bool) {
	if os.Getenv(EnvNoCrossUser) != "" {
		return "", false
	}

	return OwnerOf(path)
}

// OwnerOf is CrossUserOwner without the EnvNoCrossUser opt-out, for callers
// that need to *explain* a result rather than act on it — notably reporting
// why liveness could not be determined once the opt-out has suppressed the
// re-exec that would have determined it.
func OwnerOf(path string) (owner string, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}

	if int(stat.Uid) == os.Geteuid() {
		return "", false
	}

	u, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		return "", false
	}

	return u.Username, true
}

// ReadFileCrossUser reads file, falling back to `sudo -n -u <owner> cat` when
// a direct read is refused and the containing directory belongs to another
// user.
//
// Without this, a VM started by a service user has an instance directory
// (mode 0700) that an interactive caller cannot traverse, so reading the
// instance config fails — and callers that ignore that error report
// *confidently wrong* values rather than failing. `colima status` claimed
// driver "QEMU" for a VM actually running on macOS Virtualization.Framework,
// with an empty mountType, purely because the config read was refused.
//
// The original read error is returned when the fallback is unavailable or
// itself fails, so the caller sees the real reason and never a sudo artifact.
func ReadFileCrossUser(file string) ([]byte, error) {
	b, err := os.ReadFile(file)
	if err == nil {
		return b, nil
	}

	owner, ok := CrossUserOwner(filepath.Dir(file))
	if !ok {
		return nil, err
	}

	out, sudoErr := exec.Command("sudo", "-n", "-u", owner, "cat", file).Output()
	if sudoErr != nil {
		return nil, err
	}

	return out, nil
}
