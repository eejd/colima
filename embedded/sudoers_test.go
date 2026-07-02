package embedded

import (
	"strings"
	"testing"
)

func TestRequiredGrants(t *testing.T) {
	template := `# starting vmnet daemon
%staff ALL=(root:wheel) NOPASSWD:NOSETENV: /opt/colima/bin/socket_vmnet --vmnet-mode shared --socket-group staff --vmnet-gateway 192.168.106.1 --vmnet-dhcp-end 192.168.106.254 *
%staff ALL=(root:wheel) NOPASSWD:NOSETENV: /opt/colima/bin/socket_vmnet --vmnet-mode bridged --socket-group staff *
# terminating vmnet daemon
%staff ALL=(root:wheel) NOPASSWD:NOSETENV: /usr/bin/pkill -F /opt/colima/run/*.pid
`
	got := requiredGrants(template)
	want := []string{
		"/opt/colima/bin/socket_vmnet --vmnet-mode shared --socket-group staff --vmnet-gateway 192.168.106.1 --vmnet-dhcp-end 192.168.106.254 *",
		"/opt/colima/bin/socket_vmnet --vmnet-mode bridged --socket-group staff *",
		"/usr/bin/pkill -F /opt/colima/run/*.pid",
	}
	if len(got) != len(want) {
		t.Fatalf("requiredGrants() = %d entries, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("requiredGrants()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSudoListSatisfiesGrants(t *testing.T) {
	template := `%staff ALL=(root:wheel) NOPASSWD:NOSETENV: /opt/colima/bin/socket_vmnet --vmnet-mode bridged --socket-group staff *
%staff ALL=(root:wheel) NOPASSWD:NOSETENV: /usr/bin/pkill -F /opt/colima/run/*.pid
`
	// Reproduces the tag-reordered form sudo -l actually prints (observed:
	// "(root : wheel) NOSETENV: NOPASSWD: <cmd>" vs the template's
	// "NOPASSWD:NOSETENV: <cmd>") -- the match must be tag-order-agnostic.
	sudoListOutput := `User hive may run the following commands on host:
    (root : wheel) NOSETENV: NOPASSWD: /opt/colima/bin/socket_vmnet --vmnet-mode bridged --socket-group staff *
    (root : wheel) NOSETENV: NOPASSWD: /usr/bin/pkill -F /opt/colima/run/*.pid
`
	for _, grant := range requiredGrants(template) {
		if !strings.Contains(sudoListOutput, grant) {
			t.Errorf("expected grant %q to be present in sudo -l output", grant)
		}
	}
}
