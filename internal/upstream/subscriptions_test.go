package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestSubscriptionUsageReadsProviderMetadataWithoutInference(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Context().Value(proxyContextKey{}) != "http://proxy.example.test:8080" {
					t.Error("quota request lost the account proxy")
				}
				if r.Header.Get("Authorization") != "Bearer sk-ant-oat01-synthetic-access" {
					t.Error("quota request lost the subscription access token")
				}
				body := `{"five_hour":{"utilization":25,"resets_at":"2030-01-01T00:00:00Z"},"seven_day":{"utilization":null,"resets_at":null},"seven_day_sonnet":{"utilization":100,"resets_at":"2030-01-02T00:00:00Z"},"extra_usage":{"used_credits":123,"secret":"synthetic-private"}}`
				if provider == "claude" {
					if r.Method != http.MethodGet || r.URL.Host != "api.anthropic.com" || r.URL.Path != "/api/oauth/usage" || !strings.Contains(r.Header.Get("Anthropic-Beta"), "oauth-2025-04-20") {
						t.Error("Claude usage used an inference endpoint or omitted OAuth metadata")
					}
				} else {
					if r.Method != http.MethodPost || r.URL.Host != "cloudcode-pa.googleapis.com" || r.URL.Path != "/v1internal:fetchAvailableModels" {
						t.Error("Antigravity usage used an inference endpoint")
					}
					var input map[string]string
					if json.NewDecoder(r.Body).Decode(&input) != nil || input["project"] != "synthetic-project" || len(input) != 1 {
						t.Error("model quota request omitted its project or sent generation data")
					}
					body = `{"models":{"synthetic-model":{"quotaInfo":{"remainingFraction":0.75,"resetTime":"2030-01-01T00:00:00Z"}},"synthetic-unknown":{"quotaInfo":{}},"synthetic-internal":{"isInternal":true,"quotaInfo":{"remainingFraction":0}}},"secret":"synthetic-private"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			// Inspect proxy selection at the injected transport without opening a real proxy connection.
			client.http.Transport = client.http.Transport.(*proxyTransport).base
			defer client.Close()
			c := accounts.Credential{Provider: provider, AccessToken: "sk-ant-oat01-synthetic-access", ExpiresAt: time.Now().Add(time.Hour).Unix(), ProxyURL: "http://proxy.example.test:8080", Metadata: map[string]json.RawMessage{"project_id": json.RawMessage(`"synthetic-project"`)}}
			value, err := client.Usage(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || value.UpdatedAt <= 0 || len(value.Limits) != 2 {
				t.Fatalf("unexpected quota reads or limits: calls=%d limits=%d", calls, len(value.Limits))
			}
			windows := value.Limits[0].Windows
			if len(windows) == 0 || windows[0].UsedPercent == nil || *windows[0].UsedPercent != 25 || windows[0].ResetAt == nil || *windows[0].ResetAt != 1893456000 {
				t.Fatal("reported usage or reset time was lost")
			}
			if provider == "claude" {
				if len(windows) != 2 || windows[1].UsedPercent != nil || windows[1].ResetAt != nil || windows[0].WindowSeconds == nil || *windows[0].WindowSeconds != 18000 {
					t.Fatal("missing Claude window data became a fabricated value")
				}
			} else if value.Limits[1].Windows[0].UsedPercent != nil || value.Limits[1].Windows[0].ResetAt != nil || value.Limits[0].Name != "synthetic-model" {
				t.Fatal("model identity or unknown Antigravity quota was lost")
			}
			encoded, _ := json.Marshal(value)
			if strings.Contains(string(encoded), "synthetic-private") || value.ResetCredits != nil {
				t.Fatal("unrelated upstream metadata leaked into subscription usage")
			}
		})
	}
}

func TestSubscriptionUsageRejectsInvalidOrOversizedMetadata(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		for _, test := range []struct{ name, body string }{{"null", `null`}, {"missing quota fields", `{}`}, {"oversized", strings.Repeat("x", (2<<20)+1)}} {
			t.Run(provider+"/"+test.name, func(t *testing.T) {
				client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
				}))
				defer client.Close()
				_, err := client.Usage(context.Background(), accounts.Credential{Provider: provider, AccessToken: "synthetic-access"})
				if !errors.Is(err, ErrResponse) {
					t.Fatalf("invalid subscription metadata accepted: %v", err)
				}
			})
		}
	}
}

func TestSubscriptionUsagePreservesSanitizedUpstreamFailures(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(`{"error":"synthetic-private"}`))}, nil
		}))
		defer client.Close()
		_, err := client.Usage(context.Background(), accounts.Credential{Provider: provider, AccessToken: "synthetic-access"})
		var upstream *UpstreamError
		if !errors.As(err, &upstream) || upstream.Status != 429 || upstream.RetryAfter != "120" || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatalf("quota failure lost its safe status and retry deadline: %v", err)
		}
	}
}

func TestSubscriptionUsagePreservesZeroUnknownAndResetFormats(t *testing.T) {
	for _, test := range []struct {
		name, provider, body string
		used                 *float64
		reset                *int64
	}{
		{"Claude exhausted", "claude", `{"five_hour":{"utilization":100}}`, ptrFloat64(100), nil},
		{"Claude overage", "claude", `{"seven_day":{"utilization":102}}`, ptrFloat64(102), nil},
		{"Claude unknown", "claude", `{"five_hour":{}}`, nil, nil},
		{"Antigravity full", "antigravity", `{"models":{"synthetic-model":{"quotaInfo":{"remainingFraction":1,"resetTime":1893456000}}}}`, ptrFloat64(0), ptrInt64(1893456000)},
		{"Antigravity exhausted", "antigravity", `{"models":{"synthetic-model":{"quotaInfo":{"remainingFraction":0,"resetTime":"1893456000000"}}}}`, ptrFloat64(100), ptrInt64(1893456000)},
		{"Antigravity unknown", "antigravity", `{"models":{"synthetic-model":{}}}`, nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			defer client.Close()
			value, err := client.Usage(context.Background(), accounts.Credential{Provider: test.provider, AccessToken: "synthetic-access"})
			if err != nil || len(value.Limits) != 1 || len(value.Limits[0].Windows) != 1 {
				t.Fatalf("quota windows: %+v, %v", value, err)
			}
			window := value.Limits[0].Windows[0]
			if (window.UsedPercent == nil) != (test.used == nil) || window.UsedPercent != nil && *window.UsedPercent != *test.used ||
				(window.ResetAt == nil) != (test.reset == nil) || window.ResetAt != nil && *window.ResetAt != *test.reset {
				t.Fatalf("reported quota became fabricated: %+v", window)
			}
		})
	}
}

func TestSubscriptionUsageRejectsInvalidQuotaValues(t *testing.T) {
	for _, test := range []struct{ provider, body string }{
		{"claude", `{"five_hour":{"utilization":-1}}`},
		{"claude", `{"five_hour":{"resets_at":"invalid-time"}}`},
		{"antigravity", `{"models":{"synthetic-model":{"quotaInfo":{"remainingFraction":1.5}}}}`},
		{"antigravity", `{"models":{"synthetic-model":{"quotaInfo":{"remainingFraction":-0.5}}}}`},
		{"antigravity", `{"models":{"synthetic-model":{"quotaInfo":{"resetTime":-1}}}}`},
	} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.body))}, nil
		}))
		defer client.Close()
		if _, err := client.Usage(context.Background(), accounts.Credential{Provider: test.provider}); !errors.Is(err, ErrResponse) {
			t.Fatalf("invalid quota accepted: %v", err)
		}
	}
}

func ptrFloat64(value float64) *float64 { return &value }
