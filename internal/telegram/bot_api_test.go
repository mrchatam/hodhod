package telegram

import (
	"errors"
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
	if !strings.Contains(err.Error(), "PROXY_URL") {
		t.Fatalf("unexpected: %v", err)
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
