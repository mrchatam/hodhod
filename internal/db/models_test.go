package db

import "testing"

func TestAgentPermissions_Has(t *testing.T) {
	tests := []struct {
		name string
		p    AgentPermissions
		perm Perm
		want bool
	}{
		{"create granted", AgentPermissions{CreateUser: true}, PermCreateUser, true},
		{"create denied", AgentPermissions{ViewOnly: true}, PermCreateUser, false},
		{"view only no delete", AgentPermissions{ViewOnly: true, DeleteUser: false}, PermDeleteUser, false},
		{"manage bot", AgentPermissions{ManageBot: true}, PermManageBot, true},
		{"nil perm", AgentPermissions{}, Perm("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Has(tt.perm); got != tt.want {
				t.Fatalf("Has(%q) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestAgentPermissions_nilReceiver(t *testing.T) {
	var p *AgentPermissions
	if p.Has(PermCreateUser) {
		t.Fatal("nil permissions should deny")
	}
}
