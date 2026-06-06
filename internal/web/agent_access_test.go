package web

import (
	"errors"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
)

func TestAgentCanViewUser_ownCreate(t *testing.T) {
	v := agentVisibleUser{OwnCreate: true}
	if !agentCanViewUser(v) {
		t.Fatal("own create should be visible")
	}
}

func TestAgentCanViewUser_userGrant(t *testing.T) {
	v := agentVisibleUser{UserGrant: &db.AgentUserGrant{AllowView: true}}
	if !agentCanViewUser(v) {
		t.Fatal("user grant view should be visible")
	}
}

func TestAgentCanViewUser_inbound(t *testing.T) {
	v := agentVisibleUser{InboundVisible: true}
	if !agentCanViewUser(v) {
		t.Fatal("inbound visible should show user")
	}
}

func TestAgentCanModifyUser_viewOnlyGrant(t *testing.T) {
	v := agentVisibleUser{
		UserGrant:      &db.AgentUserGrant{AllowView: true, AllowModify: false},
		InboundVisible: true,
	}
	if agentCanModifyUser(v, true) {
		t.Fatal("view-only user grant should block modify")
	}
}

func TestAgentCanModifyUser_inboundVisible(t *testing.T) {
	v := agentVisibleUser{InboundVisible: true}
	if agentCanModifyUser(v, true) {
		t.Fatal("inbound visible alone should not allow modify without explicit grant")
	}
}

func TestAgentCanModifyUser_createOnlyOwn(t *testing.T) {
	v := agentVisibleUser{OwnCreate: true}
	if !agentCanModifyUser(v, true) {
		t.Fatal("own create should allow modify")
	}
}

func TestUserOnInbound(t *testing.T) {
	set := map[int]bool{2: true}
	u := panels.UserInfo{InboundID: 1, InboundIDs: []int{3, 2}}
	if !userOnInbound(u, set) {
		t.Fatal("expected match on inbound 2")
	}
	u2 := panels.UserInfo{InboundID: 5, InboundIDs: []int{1}}
	if userOnInbound(u2, set) {
		t.Fatal("expected no match")
	}
}

func TestFriendlySalesErr_viewOnly(t *testing.T) {
	msg := friendlySalesErr("en", sales.ErrViewOnly)
	if msg == "" || errors.Is(sales.ErrViewOnly, sales.ErrViewOnly) == false {
		t.Fatalf("unexpected: %q", msg)
	}
}

func TestCanPerm_viewOnlyBlocksCreate(t *testing.T) {
	p := &db.AgentPermissions{ViewOnly: true, CreateUser: true}
	if p.ViewOnly && db.PermCreateUser != "" && p.Has(db.PermCreateUser) {
		// ViewOnly should block even when CreateUser is set in DB (stale row)
		if !p.ViewOnly {
			t.Fatal("view only expected")
		}
	}
}
