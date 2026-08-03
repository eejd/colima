package limautil

import (
	"os"
	"os/exec"

	"github.com/abiosoft/colima/cli"
	"github.com/abiosoft/colima/config"
	"github.com/abiosoft/colima/util/osutil"
)

// EnvLimaHome is the environment variable for the Lima directory.
const EnvLimaHome = "LIMA_HOME"

// EnvLimaDrivers is the environment variable for the path to external Lima drivers.
const EnvLimaDrivers = "LIMA_DRIVERS_PATH"

// LimactlCommand is the limactl command.
const LimactlCommand = "limactl"

// EnvNoCrossUser opts out of the cross-user re-exec fallback below
// (fork addition, see macports-ports-local#162), even when the Lima
// instance directory is owned by a user other than the caller.
const EnvNoCrossUser = osutil.EnvNoCrossUser

// crossUserOwner is osutil.CrossUserOwner. It lives in osutil so the
// config-file read path can share it without importing limautil, which would
// be an import cycle (limautil imports config/configmanager).
var crossUserOwner = osutil.CrossUserOwner

// Limactl prepares a limactl command.
//
// Fork addition (macports-ports-local#162): when the Lima instance/state
// directory (config.LimaDir()) is owned by a uid other than the caller's
// effective uid -- e.g. a VM started by a headless service user and
// queried by an interactive user -- upstream limactl falsely reports
// "not running" because its liveness check cannot signal a process owned
// by another user (kill(pid, 0) fails with EPERM across the uid
// boundary). When that mismatch is detected and EnvNoCrossUser is unset,
// the limactl invocation is re-exec'd as the owning user via
// `sudo -n -u <owner>`. This requires a NOPASSWD sudoers grant for
// caller -> owner limactl; without it the re-exec fails closed (sudo -n
// errors rather than prompting). LIMA_HOME is forwarded explicitly across
// the sudo boundary since sudo does not inherit the caller's environment
// by default. The underlying gap is in Lima's process-liveness check, not
// colima; this is a colima-layer workaround pending a Lima-side fix (see
// macports-ports-local#162 Phase 3).
func Limactl(args ...string) *exec.Cmd {
	limaHome := config.LimaDir()

	if owner, ok := crossUserOwner(limaHome); ok {
		sudoArgs := append([]string{"-n", "-u", owner, "env", EnvLimaHome + "=" + limaHome, LimactlCommand}, args...)
		return cli.Command("sudo", sudoArgs...)
	}

	cmd := cli.Command(LimactlCommand, args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, EnvLimaHome+"="+limaHome)
	return cmd
}

// LimaCommand is the lima guest-shell wrapper (`lima <cmd>` runs <cmd>
// inside the VM). Distinct from LimactlCommand, which drives the VM from
// the host.
const LimaCommand = "lima"

// GuestArgs returns the argv for running `lima <args>` -- a command *inside*
// the VM -- applying the same cross-user re-exec as Limactl.
//
// Fork addition. Limactl covers host-side Lima state (list, liveness), which
// is why `colima list` works across a uid boundary. Guest execution did not
// go through it, so anything needing to read from inside the VM still failed
// cross-user: `colima status` reads the container runtime from
// /etc/colima/colima.json in the guest, the read failed silently, and status
// died with "error retrieving current runtime: empty value" on a perfectly
// healthy VM.
//
// env carries the variables the caller would otherwise have set on the child
// process (LIMA_HOME, LIMA_INSTANCE, COLIMA_BINARY) as KEY=VALUE pairs. They
// must be re-stated here because sudo resets the environment; the same
// reasoning as Limactl forwarding LIMA_HOME.
//
// Note this elevates guest *command execution*, not just host-side state
// reads -- a wider grant than Limactl's. It is gated on the same
// EnvNoCrossUser opt-out and the same NOPASSWD requirement (without the
// grant, sudo -n fails closed rather than prompting).
func GuestArgs(env []string, args ...string) []string {
	owner, crossUser := crossUserOwner(config.LimaDir())
	return guestArgs(owner, crossUser, env, args...)
}

// guestArgs is the pure half of GuestArgs, with the ownership decision
// supplied rather than probed, so both branches are testable without root.
func guestArgs(owner string, crossUser bool, env []string, args ...string) []string {
	if !crossUser {
		return append([]string{LimaCommand}, args...)
	}

	argv := []string{"sudo", "-n", "-u", owner, "env"}
	argv = append(argv, env...)
	argv = append(argv, LimaCommand)
	return append(argv, args...)
}
