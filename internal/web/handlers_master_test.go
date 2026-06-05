package web

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestDeriveViewOnly(t *testing.T) {
	p := &db.AgentPermissions{
		AgentID: 1, CreateUser: true, ModifyUser: false,
	}
	p.ViewOnly = !(p.CreateUser || p.ModifyUser || p.AddTime || p.AddVolume ||
		p.ResetUsage || p.DisableEnable || p.DeleteUser || p.ManageBot || p.ManagePlans)
	if p.ViewOnly {
		t.Fatal("should not be view-only when create_user is set")
	}
	p2 := &db.AgentPermissions{AgentID: 1}
	p2.ViewOnly = !(p2.CreateUser || p2.ModifyUser || p2.AddTime || p2.AddVolume ||
		p2.ResetUsage || p2.DisableEnable || p2.DeleteUser || p2.ManageBot || p2.ManagePlans)
	if !p2.ViewOnly {
		t.Fatal("should be view-only when no capabilities")
	}
}
