package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

// A genuine cross-uid scenario needs root to set up (chown to another user),
// so it is left to live two-user validation; these cover what can be
// exercised as the test-runner user.

func TestCrossUserOwner(t *testing.T) {
	t.Run("a self-owned dir is not cross-user", func(t *testing.T) {
		dir := t.TempDir()

		if _, ok := CrossUserOwner(dir); ok {
			t.Errorf("CrossUserOwner(%q) reported cross-user for a self-owned dir", dir)
		}
	})

	t.Run("a missing path is not cross-user", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does-not-exist")

		if _, ok := CrossUserOwner(dir); ok {
			t.Errorf("CrossUserOwner(%q) reported cross-user for a missing path", dir)
		}
	})

	t.Run("EnvNoCrossUser opts out", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(EnvNoCrossUser, "1")

		if _, ok := CrossUserOwner(dir); ok {
			t.Errorf("CrossUserOwner(%q) did not honour the %s opt-out", dir, EnvNoCrossUser)
		}
	})
}

// TestOwnerOfIgnoresOptOut is what lets `colima status` say "cannot
// determine" instead of "not running" once the opt-out has suppressed the
// check that would have determined it. If OwnerOf honoured the opt-out too,
// the diagnostic would be silently unavailable exactly when it is needed.
func TestOwnerOfIgnoresOptOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvNoCrossUser, "1")

	// Self-owned, so ok is false either way; the point is that OwnerOf does
	// not short-circuit on the env var before it has looked at the path.
	if _, ok := OwnerOf(dir); ok {
		t.Errorf("OwnerOf(%q) reported another owner for a self-owned dir", dir)
	}

	missing := filepath.Join(dir, "nope")
	if _, ok := OwnerOf(missing); ok {
		t.Errorf("OwnerOf(%q) reported an owner for a missing path", missing)
	}
}

func TestReadFileCrossUser(t *testing.T) {
	t.Run("a readable file is read directly", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "conf.yaml")
		want := "vmType: vz\n"
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := ReadFileCrossUser(path)
		if err != nil {
			t.Fatalf("ReadFileCrossUser() error = %v", err)
		}
		if string(got) != want {
			t.Errorf("ReadFileCrossUser() = %q, want %q", got, want)
		}
	})

	t.Run("a missing file returns the original error, not a sudo artifact", func(t *testing.T) {
		// The caller must see why the read failed. Substituting a sudo
		// failure here would replace a clear "no such file" with an opaque
		// exit status, which is the error-swallowing this fork is undoing.
		path := filepath.Join(t.TempDir(), "absent.yaml")

		_, err := ReadFileCrossUser(path)
		if err == nil {
			t.Fatal("ReadFileCrossUser() succeeded for a missing file")
		}
		if !os.IsNotExist(err) {
			t.Errorf("ReadFileCrossUser() error = %v, want a not-exist error", err)
		}
	})

	t.Run("no fallback is attempted for a self-owned directory", func(t *testing.T) {
		// Guards against shelling out to sudo on every ordinary miss.
		t.Setenv(EnvNoCrossUser, "1")
		path := filepath.Join(t.TempDir(), "absent.yaml")

		if _, err := ReadFileCrossUser(path); !os.IsNotExist(err) {
			t.Errorf("ReadFileCrossUser() error = %v, want a not-exist error", err)
		}
	})
}
