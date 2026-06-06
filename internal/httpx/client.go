package httpx

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mrchatam/hodhod/internal/debuglog"
	"golang.org/x/net/proxy"
)

// Config for building an outbound HTTP client.
type Config struct {
	ProxyURL string
	Timeout  time.Duration
}

// New returns a proxy-aware HTTP client. Empty ProxyURL uses direct connection.
func New(cfg Config) (*http.Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if cfg.ProxyURL != "" {
		dialer, err := socksDialer(cfg.ProxyURL)
		if err != nil {
			return nil, err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
		// #region agent log
		host := proxyHost(cfg.ProxyURL)
		debuglog.Write("A", "httpx/client.go:New", "outbound client with proxy", map[string]any{
			"proxyHost": host, "scheme": proxyScheme(cfg.ProxyURL), "timeoutSec": cfg.Timeout.Seconds(),
		})
		// #endregion
	} else {
		// #region agent log
		debuglog.Write("A", "httpx/client.go:New", "outbound client direct (no proxy)", map[string]any{
			"timeoutSec": cfg.Timeout.Seconds(),
		})
		// #endregion
	}
	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}, nil
}

func socksDialer(proxyURL string) (proxy.Dialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("httpx: parse proxy URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "socks5" && scheme != "socks5h" {
		return nil, fmt.Errorf("httpx: unsupported proxy scheme %q", u.Scheme)
	}
	host := u.Host
	if host == "" {
		return nil, fmt.Errorf("httpx: empty proxy host")
	}
	var auth *proxy.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pass}
	}
	// socks5h resolves DNS through proxy
	return proxy.SOCKS5("tcp", host, auth, proxy.Direct)
}

func proxyHost(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func proxyScheme(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return ""
	}
	return u.Scheme
}
