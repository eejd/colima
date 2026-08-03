package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

// foreignDir returns a directory owned by a uid other than the test runner's.
// Creating one would need root (chown), but every unix already ships one:
// /usr is root-owned. Without this the cross-user branch could only be
// asserted negatively, and a helper that short-circuited before ever calling
// stat would pass every test.
func foreignDir(t *testing.T) string {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("running as root: no directory is foreign")
	}

	const dir = "/usr"
	info, err := os.Stat(dir)
	if err != nil {
		t.Skipf("cannot stat %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Skipf("%s is not a directory", dir)
	}

	return dir
}

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

	t.Run("a foreign-owned dir is cross-user", func(t *testing.T) {
		dir := foreignDir(t)

		owner, ok := CrossUserOwner(dir)
		if !ok {
			t.Fatalf("CrossUserOwner(%q) = _, false; want the owning user", dir)
		}
		if owner == "" {
			t.Errorf("CrossUserOwner(%q) returned an empty owner", dir)
		}
	})

	t.Run("EnvNoCrossUser opts out of a genuinely foreign dir", func(t *testing.T) {
		// Asserted against a foreign dir, not a self-owned one: against a
		// self-owned dir the result is false with or without the opt-out, so
		// the test would pass for a reason unrelated to the opt-out.
		dir := foreignDir(t)
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
	dir := foreignDir(t)
	t.Setenv(EnvNoCrossUser, "1")

	owner, ok := OwnerOf(dir)
	if !ok {
		t.Fatalf("OwnerOf(%q) = _, false with %s set; want the owner regardless of the opt-out", dir, EnvNoCrossUser)
	}
	if owner == "" {
		t.Errorf("OwnerOf(%q) returned an empty owner", dir)
	}

	// Sanity: the opt-out is in force, so the acting-on-it helper still says no.
	if _, ok := CrossUserOwner(dir); ok {
		t.Errorf("CrossUserOwner(%q) ignored the %s opt-out", dir, EnvNoCrossUser)
	}

	missing := filepath.Join(t.TempDir(), "nope")
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

	t.Run("no fallback for a self-owned directory", func(t *testing.T) {
		// No opt-out set here on purpose: the ownership check alone must
		// keep an ordinary missing-file miss from shelling out to sudo.
		path := filepath.Join(t.TempDir(), "absent.yaml")

		if _, err := ReadFileCrossUser(path); !os.IsNotExist(err) {
			t.Errorf("ReadFileCrossUser() error = %v, want a not-exist error", err)
		}
	})

	t.Run("the opt-out suppresses the fallback for a foreign directory", func(t *testing.T) {
		dir := foreignDir(t)
		t.Setenv(EnvNoCrossUser, "1")
		path := filepath.Join(dir, "definitely-absent.yaml")

		if _, err := ReadFileCrossUser(path); !os.IsNotExist(err) {
			t.Errorf("ReadFileCrossUser() error = %v, want a not-exist error", err)
		}
	})
}
