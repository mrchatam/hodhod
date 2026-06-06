package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestUserRowTemplate_agentScopeUsesAgentURLs(t *testing.T) {
	pages, _, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tpl := pages["agent_panel_customers"]
	panel := &db.Panel{ID: 5, Name: "P1"}
	data := map[string]any{
		"Lang": "en", "CSRF": "tok", "Panel": panel,
		"Row": PanelUserRow{Username: "user@test", PanelID: 5, CanModify: true, Enabled: true},
		"Scope": "agent-panel", "ShowOnline": false, "ShowSource": false, "ShowInbound": false,
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "user_row", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "/agent/panels/5/users/user@test/modify") {
		t.Fatalf("expected agent modify URL, got: %s", out)
	}
	if strings.Contains(out, "/master/panels/") {
		t.Fatalf("master URL leaked in agent row: %s", out)
	}
}

func TestUserRowTemplate_masterScopeUsesMasterURLs(t *testing.T) {
	pages, _, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tpl := pages["panel_edit"]
	panel := &db.Panel{ID: 3, Name: "P1"}
	data := map[string]any{
		"Lang": "en", "CSRF": "tok", "Panel": panel, "IsMaster": true,
		"Agents": []db.Agent{{ID: 1, Name: "A"}},
		"Row": PanelUserRow{Username: "user@test", PanelID: 3, Enabled: true},
		"Scope": "master-panel", "ShowOnline": true, "ShowSource": true, "ShowInbound": true,
		"InboundTagMap": map[int]string{},
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "user_row", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "/master/panels/3/users/user@test/modify") {
		t.Fatalf("expected master modify URL, got: %s", out)
	}
}
