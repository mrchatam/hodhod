package web

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestGrantToggleCombinations(t *testing.T) {
	cases := []struct {
		name    string
		v       agentVisibleUser
		canView bool
		canMod  bool
	}{
		{
			name:    "create only own",
			v:       agentVisibleUser{OwnCreate: true},
			canView: true, canMod: true,
		},
		{
			name:    "inbound view no create grant",
			v:       agentVisibleUser{InboundVisible: true},
			canView: true, canMod: true,
		},
		{
			name: "inbound create on view off pre-existing hidden",
			v:    agentVisibleUser{},
			canView: false, canMod: false,
		},
		{
			name: "user view modify off",
			v: agentVisibleUser{
				UserGrant: &db.AgentUserGrant{AllowView: true, AllowModify: false},
			},
			canView: true, canMod: false,
		},
		{
			name: "user grant view without inbound",
			v: agentVisibleUser{
				UserGrant: &db.AgentUserGrant{AllowView: true, AllowModify: true},
			},
			canView: true, canMod: true,
		},
		{
			name:    "all toggles off",
			v:       agentVisibleUser{},
			canView: false, canMod: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotView := agentCanViewUser(tc.v)
			gotMod := agentCanModifyUser(tc.v, true)
			if gotView != tc.canView {
				t.Fatalf("view: got %v want %v", gotView, tc.canView)
			}
			if gotMod != tc.canMod {
				t.Fatalf("modify: got %v want %v", gotMod, tc.canMod)
			}
		})
	}
}
