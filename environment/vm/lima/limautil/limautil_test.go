package limautil

import (
	"path/filepath"
	"slices"
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

// TestGuestArgs covers the guest-exec half of the cross-user re-exec. Guest
// execution used to bypass it entirely, which is why `colima list` worked
// across a uid boundary while `colima status` died with "error retrieving
// current runtime: empty value" -- status additionally reads
// /etc/colima/colima.json from inside the VM.
func TestGuestArgs(t *testing.T) {
	env := []string{"LIMA_HOME=/colima/dotcolima/_lima", "LIMA_INSTANCE=colima-hive", "COLIMA_BINARY=/opt/local/bin/colima"}

	t.Run("same user runs lima directly", func(t *testing.T) {
		got := guestArgs("", false, env, "sudo", "cat", "/etc/colima/colima.json")
		want := []string{"lima", "sudo", "cat", "/etc/colima/colima.json"}

		if !slices.Equal(got, want) {
			t.Errorf("guestArgs() = %v, want %v", got, want)
		}
	})

	t.Run("cross user re-execs via sudo with every env var forwarded", func(t *testing.T) {
		got := guestArgs("hive", true, env, "sudo", "cat", "/etc/colima/colima.json")
		want := []string{
			"sudo", "-n", "-u", "hive", "env",
			"LIMA_HOME=/colima/dotcolima/_lima", "LIMA_INSTANCE=colima-hive", "COLIMA_BINARY=/opt/local/bin/colima",
			"lima", "sudo", "cat", "/etc/colima/colima.json",
		}

		if !slices.Equal(got, want) {
			t.Errorf("guestArgs() = %v, want %v", got, want)
		}
	})

	t.Run("every env var is forwarded, not just LIMA_HOME", func(t *testing.T) {
		// sudo resets the environment, so an env var the caller attached via
		// host.WithEnv but that is not re-stated here is silently lost -- the
		// child would run against the wrong Lima instance.
		got := guestArgs("hive", true, env, "uname")

		for _, e := range env {
			if !slices.Contains(got, e) {
				t.Errorf("guestArgs() dropped %q across the sudo boundary: %v", e, got)
			}
		}
	})

	t.Run("caller args are not mutated", func(t *testing.T) {
		// guestArgs prepends to a fresh slice; appending onto the caller's
		// backing array would corrupt a reused args slice.
		args := []string{"uname", "-a"}
		_ = guestArgs("hive", true, env, args...)

		if !slices.Equal(args, []string{"uname", "-a"}) {
			t.Errorf("guestArgs() mutated caller args: %v", args)
		}
	})

	t.Run("GuestArgs uses the real ownership probe", func(t *testing.T) {
		// The test runner owns its own Lima dir, so this must take the
		// direct branch -- i.e. the common case is unchanged from upstream.
		got := GuestArgs(env, "uname")

		if got[0] != LimaCommand {
			t.Errorf("GuestArgs()[0] = %q, want %q (no re-exec for a self-owned dir)", got[0], LimaCommand)
		}
	})

	t.Run("EnvNoCrossUser opts out", func(t *testing.T) {
		t.Setenv(EnvNoCrossUser, "1")

		if got := GuestArgs(env, "uname"); got[0] != LimaCommand {
			t.Errorf("GuestArgs()[0] = %q, want %q with %s set", got[0], LimaCommand, EnvNoCrossUser)
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
