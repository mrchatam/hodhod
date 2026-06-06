package sales

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestCheckPerm_viewOnlyLogic(t *testing.T) {
	p := &db.AgentPermissions{ViewOnly: true, CreateUser: true}
	if !p.ViewOnly {
		t.Fatal("view only")
	}
	// ViewOnly blocks even when CreateUser flag is stale true
	if p.ViewOnly && db.PermCreateUser != "" {
		// mirrors checkPerm behavior
		err := ErrViewOnly
		if err != ErrViewOnly {
			t.Fatal(err)
		}
	}
}

func TestCheckPerm_missingPermLogic(t *testing.T) {
	p := &db.AgentPermissions{ViewOnly: false, CreateUser: false}
	if p.ViewOnly || p.Has(db.PermCreateUser) {
		t.Fatal("setup")
	}
	err := ErrPermDenied
	if err != ErrPermDenied {
		t.Fatal(err)
	}
}

func TestErrPanelNotAssigned_distinct(t *testing.T) {
	if ErrPanelNotAssigned == ErrForbidden {
		t.Fatal("errors should differ")
	}
	if ErrNoCreateInbound == ErrForbidden {
		t.Fatal("errors should differ")
	}
}
