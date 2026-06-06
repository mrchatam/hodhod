package telegram

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFriendlyTokenError_notFound(t *testing.T) {
	err := FriendlyTokenError(errors.New("call getMe, not found, Not Found"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestFriendlyTokenError_network(t *testing.T) {
	err := FriendlyTokenError(errors.New("dial tcp: connection refused"))
	if !strings.Contains(err.Error(), "cannot reach Telegram API") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestFriendlyTokenError_deadline(t *testing.T) {
	err := FriendlyTokenError(errors.New(`Post "https://api.telegram.org/bot/getMe": context deadline exceeded`))
	if !strings.Contains(err.Error(), "HODHOD_HOST_NETWORK") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestFriendlyTokenError_dialTimeout(t *testing.T) {
	err := FriendlyTokenError(errors.New("dial tcp 149.154.166.110:443: i/o timeout"))
	if !strings.Contains(err.Error(), "HODHOD_HOST_NETWORK") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestGetMeViaHTTP_ok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123456:ABC/getMe" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"testbot"}}`))
	}))
	defer srv.Close()

	old := telegramAPIBase
	telegramAPIBase = srv.URL
	t.Cleanup(func() { telegramAPIBase = old })

	username, err := getMeViaHTTP(t.Context(), srv.Client(), "123456:ABC")
	if err != nil {
		t.Fatal(err)
	}
	if username != "testbot" {
		t.Fatalf("username: %q", username)
	}
}

func TestValidateToken_format(t *testing.T) {
	_, err := ValidateToken(t.Context(), nil, "  not-a-token  ", nil)
	if err == nil {
		t.Fatal("expected format error")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateToken_empty(t *testing.T) {
	_, err := ValidateToken(t.Context(), nil, "   ", nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected: %v", err)
	}
}
