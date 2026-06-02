package httpx

import (
	"testing"
	"time"
)

func TestNew_direct(t *testing.T) {
	c, err := New(Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil client")
	}
}

func TestNew_badProxy(t *testing.T) {
	_, err := New(Config{ProxyURL: "://bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_unsupportedScheme(t *testing.T) {
	_, err := New(Config{ProxyURL: "http://127.0.0.1:1080"})
	if err == nil {
		t.Fatal("expected error")
	}
}
