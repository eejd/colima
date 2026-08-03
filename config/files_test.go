package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withDefaultHome sets the compile-time packaging default for one test.
func withDefaultHome(t *testing.T, path string) {
	t.Helper()

	prev := defaultHome
	defaultHome = path
	t.Cleanup(func() { defaultHome = prev })
}

// withReadOnly marks the invocation read-only for one test.
func withReadOnly(t *testing.T, v bool) {
	t.Helper()

	prev := readOnly
	readOnly = v
	t.Cleanup(func() { readOnly = prev })
}

// TestConfigBaseDirResolution covers home resolution. The precedence is
// COLIMA_HOME -> defaultHome (if it exists) -> ~/.colima / XDG, and getting
// it wrong is not a cosmetic bug: a colima that resolves to the wrong home
// reports a running VM as "not running", because it is reading a different
// (or empty) state tree entirely.
func TestConfigBaseDirResolution(t *testing.T) {
	t.Run("COLIMA_HOME wins even when it does not exist yet", func(t *testing.T) {
		// Upstream gated this on os.Stat succeeding, so
		// `COLIMA_HOME=/new/path colima start` silently fell through to
		// ~/.colima and created the VM somewhere the caller never named.
		want := filepath.Join(t.TempDir(), "not-created-yet")
		t.Setenv("COLIMA_HOME", want)

		got, err := configBaseDir.dir()
		if err != nil {
			t.Fatalf("configBaseDir.dir() error = %v", err)
		}
		if got != want {
			t.Errorf("configBaseDir.dir() = %q, want %q", got, want)
		}
	})

	t.Run("COLIMA_HOME wins over defaultHome", func(t *testing.T) {
		want := t.TempDir()
		t.Setenv("COLIMA_HOME", want)
		withDefaultHome(t, t.TempDir())

		got, err := configBaseDir.dir()
		if err != nil {
			t.Fatalf("configBaseDir.dir() error = %v", err)
		}
		if got != want {
			t.Errorf("configBaseDir.dir() = %q, want COLIMA_HOME %q", got, want)
		}
	})

	t.Run("defaultHome is used when it exists and COLIMA_HOME is unset", func(t *testing.T) {
		t.Setenv("COLIMA_HOME", "")
		want := t.TempDir()
		withDefaultHome(t, want)

		got, err := configBaseDir.dir()
		if err != nil {
			t.Fatalf("configBaseDir.dir() error = %v", err)
		}
		if got != want {
			t.Errorf("configBaseDir.dir() = %q, want defaultHome %q", got, want)
		}
	})

	t.Run("a defaultHome that does not exist is ignored", func(t *testing.T) {
		// A packaged default pointing at an unmounted volume must fall back
		// to the user's home rather than resolving to a path with nothing
		// behind it.
		t.Setenv("COLIMA_HOME", "")
		absent := filepath.Join(t.TempDir(), "unmounted")
		withDefaultHome(t, absent)

		got, err := configBaseDir.dir()
		if err != nil {
			t.Fatalf("configBaseDir.dir() error = %v", err)
		}
		if got == absent {
			t.Errorf("configBaseDir.dir() = %q, want a fallback (defaultHome does not exist)", got)
		}
	})

	t.Run("an empty defaultHome leaves resolution exactly as upstream", func(t *testing.T) {
		t.Setenv("COLIMA_HOME", "")
		withDefaultHome(t, "")

		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no user home dir: %v", err)
		}
		t.Setenv("XDG_CONFIG_HOME", "")

		got, err := configBaseDir.dir()
		if err != nil {
			t.Fatalf("configBaseDir.dir() error = %v", err)
		}
		if want := filepath.Join(home, ".colima"); got != want {
			t.Errorf("configBaseDir.dir() = %q, want %q", got, want)
		}
	})
}

// TestReadOnlyDoesNotCreate is the guard for the self-inflicted half of the
// bug: `colima status` resolving a home also *created* it, and a stray empty
// ~/.colima then masks the real home for every later invocation. A query must
// never mint the directory whose absence it is reporting on.
func TestReadOnlyDoesNotCreate(t *testing.T) {
	t.Run("read-only resolution creates nothing", func(t *testing.T) {
		withReadOnly(t, true)
		target := filepath.Join(t.TempDir(), "should-not-appear")
		r := requiredDir{dir: func() (string, error) { return target, nil }}

		if got := r.Dir(); got != target {
			t.Errorf("Dir() = %q, want %q", got, target)
		}
		if _, err := os.Stat(target); err == nil {
			t.Errorf("Dir() created %q while read-only", target)
		}
	})

	t.Run("Path never creates, read-only or not", func(t *testing.T) {
		withReadOnly(t, false)
		target := filepath.Join(t.TempDir(), "should-not-appear")
		r := requiredDir{dir: func() (string, error) { return target, nil }}

		if got := r.Path(); got != target {
			t.Errorf("Path() = %q, want %q", got, target)
		}
		if _, err := os.Stat(target); err == nil {
			t.Errorf("Path() created %q", target)
		}
	})

	t.Run("writable resolution still creates", func(t *testing.T) {
		withReadOnly(t, false)
		target := filepath.Join(t.TempDir(), "created")
		r := requiredDir{dir: func() (string, error) { return target, nil }}

		if got := r.Dir(); got != target {
			t.Errorf("Dir() = %q, want %q", got, target)
		}
		if _, err := os.Stat(target); err != nil {
			t.Errorf("Dir() did not create %q: %v", target, err)
		}
	})

	t.Run("a read-only resolution is not cached for a later writable one", func(t *testing.T) {
		// Otherwise `status` running before `start` in the same process
		// would leave the path memoised as never-created, and start would
		// then operate against a directory that does not exist.
		target := filepath.Join(t.TempDir(), "created-later")
		r := requiredDir{dir: func() (string, error) { return target, nil }}

		withReadOnly(t, true)
		_ = r.Dir()
		if _, err := os.Stat(target); err == nil {
			t.Fatalf("read-only Dir() created %q", target)
		}

		readOnly = false
		if _, err := os.Stat(r.Dir()); err != nil {
			t.Errorf("writable Dir() did not create %q after a read-only call: %v", target, err)
		}
	})
}
