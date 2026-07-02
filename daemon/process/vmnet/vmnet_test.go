package vmnet

import (
	"path/filepath"
	"testing"

	"github.com/abiosoft/colima/daemon/process"
)

// TestInfoSocketOverride covers the COLIMA_VMNET_SOCKET fork override
// (macports-ports-local#168): unset must be byte-identical to upstream
// behaviour, and set must be honoured verbatim.
func TestInfoSocketOverride(t *testing.T) {
	t.Run("unset falls back to the default path", func(t *testing.T) {
		t.Setenv(EnvVmnetSocket, "")

		want := filepath.Join(process.Dir(), "vmnet.sock")
		got := Info().Socket.File()

		if got != want {
			t.Errorf("Info().Socket.File() = %q, want %q", got, want)
		}
	})

	t.Run("set overrides the default path verbatim", func(t *testing.T) {
		override := "/opt/colima/run/vmnet-hive.sock"
		t.Setenv(EnvVmnetSocket, override)

		got := Info().Socket.File()

		if got != override {
			t.Errorf("Info().Socket.File() = %q, want %q", got, override)
		}
	})
}
