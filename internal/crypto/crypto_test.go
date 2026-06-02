package crypto

import (
	"encoding/base64"
	"testing"
)

func testKey() string {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestEncryptDecrypt_roundTrip(t *testing.T) {
	b, err := NewBox(testKey())
	if err != nil {
		t.Fatal(err)
	}
	ct, err := b.Encrypt("secret-token")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := b.Decrypt(ct)
	if err != nil || pt != "secret-token" {
		t.Fatalf("got %q err %v", pt, err)
	}
}

func TestDecrypt_tampered(t *testing.T) {
	b, _ := NewBox(testKey())
	ct, _ := b.Encrypt("x")
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := b.Decrypt(tampered); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewBox_wrongKeySize(t *testing.T) {
	_, err := NewBox(base64.StdEncoding.EncodeToString([]byte("short")))
	if err == nil {
		t.Fatal("expected error")
	}
}
