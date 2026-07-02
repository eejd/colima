package limautil

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/abiosoft/colima/cli"
	"github.com/abiosoft/colima/config"
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
const EnvNoCrossUser = "COLIMA_NO_CROSS_USER"

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

// crossUserOwner reports the username owning dir when it differs from the
// caller's effective uid, so Limactl can decide whether to re-exec via
// sudo. ok is false when: the caller opted out via EnvNoCrossUser; dir
// does not exist yet (nothing to own cross-user, e.g. first `colima
// start`); the owning uid cannot be resolved to a username; or dir is
// already owned by the caller (the common case -- no re-exec needed).
func crossUserOwner(dir string) (owner string, ok bool) {
	if os.Getenv(EnvNoCrossUser) != "" {
		return "", false
	}

	info, err := os.Stat(dir)
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
