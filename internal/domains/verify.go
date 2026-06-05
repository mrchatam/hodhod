package domains

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

var ErrVerifyFailed = errors.New("domains: verification failed")

// Resolver performs DNS lookups for domain verification.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

// NetResolver uses the standard library DNS resolver.
type NetResolver struct{}

func (NetResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	var r net.Resolver
	return r.LookupTXT(ctx, name)
}

func (NetResolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	var r net.Resolver
	return r.LookupCNAME(ctx, name)
}

// Verifier checks DNS records for custom domain ownership.
type Verifier struct {
	Resolver Resolver
}

// Verify succeeds if TXT on _hodhod-verify.{domain} contains token or CNAME points to platformHost.
func (v *Verifier) Verify(ctx context.Context, domain, token, platformHost string) error {
	if v.Resolver == nil {
		v.Resolver = NetResolver{}
	}
	domain = NormalizeDomain(domain)
	platformHost = strings.ToLower(strings.TrimSpace(platformHost))
	if domain == "" || token == "" || platformHost == "" {
		return ErrVerifyFailed
	}
	txtName := "_hodhod-verify." + domain
	records, err := v.Resolver.LookupTXT(ctx, txtName)
	if err == nil {
		for _, rec := range records {
			if strings.TrimSpace(rec) == token {
				return nil
			}
		}
	}
	cname, err := v.Resolver.LookupCNAME(ctx, domain)
	if err == nil {
		cname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
		if cname == platformHost || strings.HasSuffix(cname, "."+platformHost) {
			return nil
		}
	}
	return fmt.Errorf("%w: no matching TXT or CNAME", ErrVerifyFailed)
}

// NormalizeDomain strips scheme/port and lowercases a hostname.
func NormalizeDomain(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	if len(s) > 253 || s == "" {
		return ""
	}
	if net.ParseIP(s) != nil {
		return ""
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return ""
		}
	}
	return s
}
