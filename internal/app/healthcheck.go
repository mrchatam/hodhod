package app

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// HealthCheck probes the local /healthz endpoint (used by Docker HEALTHCHECK).
// Returns 0 on success, 1 on failure.
func HealthCheck() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	var url string
	switch {
	case strings.HasPrefix(addr, "http://"), strings.HasPrefix(addr, "https://"):
		url = strings.TrimSuffix(addr, "/") + "/healthz"
	case strings.HasPrefix(addr, ":"):
		url = fmt.Sprintf("http://127.0.0.1%s/healthz", addr)
	default:
		url = fmt.Sprintf("http://%s/healthz", addr)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
