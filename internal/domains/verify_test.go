package domains

import (
	"context"
	"errors"
	"testing"
)

type mockResolver struct {
	txt   map[string][]string
	cname map[string]string
}

func (m mockResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if rec, ok := m.txt[name]; ok {
		return rec, nil
	}
	return nil, errors.New("not found")
}

func (m mockResolver) LookupCNAME(_ context.Context, name string) (string, error) {
	if c, ok := m.cname[name]; ok {
		return c, nil
	}
	return "", errors.New("not found")
}

func TestVerify_txtMatch(t *testing.T) {
	v := &Verifier{Resolver: mockResolver{txt: map[string][]string{
		"_hodhod-verify.shop.example.com": {"abc123"},
	}}}
	if err := v.Verify(context.Background(), "shop.example.com", "abc123", "main.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_cnameMatch(t *testing.T) {
	v := &Verifier{Resolver: mockResolver{cname: map[string]string{
		"shop.example.com": "main.example.com.",
	}}}
	if err := v.Verify(context.Background(), "shop.example.com", "tok", "main.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_fail(t *testing.T) {
	v := &Verifier{Resolver: mockResolver{}}
	if err := v.Verify(context.Background(), "shop.example.com", "tok", "main.example.com"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeDomain(t *testing.T) {
	if got := NormalizeDomain("HTTPS://Shop.Example.COM/"); got != "shop.example.com" {
		t.Fatalf("got %q", got)
	}
	if NormalizeDomain("127.0.0.1") != "" {
		t.Fatal("ip rejected")
	}
}
