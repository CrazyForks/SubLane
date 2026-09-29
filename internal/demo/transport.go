package demo

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Refreshable synthetic snapshots keep long-running demos useful without ever dialing a provider.
type transport struct{}

func (transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Host != "chatgpt.com" {
		return nil, errors.New("demo_network_disabled")
	}
	var value any
	switch r.URL.Path {
	case "/backend-api/wham/usage":
		value = map[string]any{"rate_limit": map[string]any{
			"allowed": true, "limit_reached": false,
			"primary_window":   map[string]any{"used_percent": 28, "limit_window_seconds": 18000, "reset_after_seconds": 10800},
			"secondary_window": map[string]any{"used_percent": 46, "limit_window_seconds": 604800, "reset_after_seconds": 345600},
		}}
	case "/backend-api/codex/models":
		value = map[string]any{"models": []map[string]any{
			{"slug": "demo-codex", "visibility": "list", "supported_in_api": true},
			{"slug": "demo-codex-mini", "visibility": "list", "supported_in_api": true},
		}}
	default:
		return nil, errors.New("demo_network_disabled")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
}
