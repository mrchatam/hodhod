package web

import (
	"net/url"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestParseUserGrantsFromForm(t *testing.T) {
	form := url.Values{}
	form.Set("user_alice%40x_view", "on")
	form.Set("user_alice%40x_modify", "on")
	form.Set("user_bob%40x_view", "on")

	grants := parseUserGrantsFromForm(form)
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %d", len(grants))
	}
	byUser := map[string]db.AgentUserGrant{}
	for _, g := range grants {
		byUser[g.PanelUsername] = g
	}
	if !byUser["alice@x"].AllowView || !byUser["alice@x"].AllowModify {
		t.Fatal("alice should have view and modify")
	}
	if !byUser["bob@x"].AllowView || byUser["bob@x"].AllowModify {
		t.Fatal("bob should have view only")
	}
}

func TestParseAccessTableUsernames(t *testing.T) {
	form := url.Values{}
	form.Set("access_user_names", "alice%40x,bob%40x")
	names := parseAccessTableUsernames(form)
	if len(names) != 2 || names[0] != "alice@x" || names[1] != "bob@x" {
		t.Fatalf("unexpected names: %v", names)
	}
}
