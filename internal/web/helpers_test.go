package web

import (
	"errors"
	"testing"
)

func TestPanelTestMessage_ok(t *testing.T) {
	ok, msg := panelTestMessage("en", nil)
	if !ok || msg == "" {
		t.Fatalf("got ok=%v msg=%q", ok, msg)
	}
}

func TestPanelTestMessage_fail(t *testing.T) {
	ok, msg := panelTestMessage("en", errors.New("login status 401"))
	if ok || msg == "" {
		t.Fatalf("got ok=%v msg=%q", ok, msg)
	}
}
