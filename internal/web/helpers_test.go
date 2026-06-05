package web

import (
	"errors"
	"strings"
	"testing"
)

func TestPanelTestMessage_ok(t *testing.T) {
	ok, msg := panelTestMessage(nil)
	if !ok || !strings.Contains(msg, "OK") {
		t.Fatalf("got ok=%v msg=%q", ok, msg)
	}
}

func TestPanelTestMessage_fail(t *testing.T) {
	ok, msg := panelTestMessage(errors.New("login status 401"))
	if ok || !strings.Contains(msg, "failed") {
		t.Fatalf("got ok=%v msg=%q", ok, msg)
	}
}
