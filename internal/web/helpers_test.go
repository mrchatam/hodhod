package web

import "testing"

func TestEncodeDecodeFlashValue(t *testing.T) {
	raw := encodeFlashValue("ok", "ذخیره شد")
	kind, msg, ok := decodeFlashCookie(raw)
	if !ok || kind != "ok" || msg != "ذخیره شد" {
		t.Fatalf("decode failed: ok=%v kind=%q msg=%q", ok, kind, msg)
	}
}

func TestDecodeFlashValue_legacyPlain(t *testing.T) {
	kind, msg, ok := decodeFlashCookie("err:plain message")
	if !ok || kind != "err" || msg != "plain message" {
		t.Fatalf("legacy decode failed: %q %q", kind, msg)
	}
}
