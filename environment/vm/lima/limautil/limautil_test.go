package limautil

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCrossUserOwner covers the cross-user detection added for
// macports-ports-local#162. A genuine cross-uid scenario needs root to set
// up (chown to another user) so is left to live two-user validation; these
// cases cover what can be exercised as the current test-runner user.
func TestCrossUserOwner(t *testing.T) {
	t.Run("dir owned by the caller is not treated as cross-user", func(t *testing.T) {
		dir := t.TempDir()

		if _, ok := crossUserOwner(dir); ok {
			t.Errorf("crossUserOwner(%q) reported cross-user for a self-owned dir", dir)
		}
	})

	t.Run("missing dir is not treated as cross-user", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does-not-exist")

		if _, ok := crossUserOwner(dir); ok {
			t.Errorf("crossUserOwner(%q) reported cross-user for a missing dir", dir)
		}
	})

	t.Run("EnvNoCrossUser opts out even for a self-owned dir", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(EnvNoCrossUser, "1")

		if _, ok := crossUserOwner(dir); ok {
			t.Errorf("crossUserOwner(%q) did not honour %s opt-out", dir, EnvNoCrossUser)
		}
	})
}

// TestLimactlUnchangedForSameUser confirms Limactl still builds a plain
// `limactl <args>` command (with LIMA_HOME forwarded) when the caller owns
// the Lima dir -- i.e. the common case is unchanged from upstream and the
// sudo re-exec path is not taken.
func TestLimactlUnchangedForSameUser(t *testing.T) {
	cmd := Limactl("list", "--json")

	if got := cmd.Args[0]; got != LimactlCommand {
		t.Errorf("Limactl() command = %q, want %q (sudo re-exec should not trigger for a self-owned dir)", got, LimactlCommand)
	}

	found := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, EnvLimaHome+"=") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Limactl() env missing %s, got: %v", EnvLimaHome, cmd.Env)
	}
}
