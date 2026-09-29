package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestGrokSubscriptionUsageReadsIncludedCreditsOnly(t *testing.T) {
	tests := []struct {
		name, body string
		percent    *float64
		seconds    int64
		reset      int64
		empty      bool
	}{
		{"weekly", `{"config":{"creditUsagePercent":37.5,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2030-01-01T00:00:00Z","end":"2030-01-08T00:00:00Z"},"productUsage":[{"product":"PRODUCT_GROK_BUILD","usagePercent":99}],"onDemandUsed":{"val":99999},"prepaidBalance":{"val":1000}}}`, grokPercent(37.5), 604800, 1894060800, false},
		{"zero", `{"config":{"creditUsagePercent":0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY"}}}`, grokPercent(0), 604800, 0, false},
		{"omitted", `{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY"},"onDemandUsed":{"val":50},"prepaidBalance":{"val":1000}}}`, nil, 604800, 0, false},
		{"legacy monthly", `{"config":{"monthlyLimit":{"val":2000},"used":{"val":500},"billingPeriodStart":"2030-01-01T00:00:00Z","billingPeriodEnd":"2030-02-01T00:00:00Z"}}`, grokPercent(25), 31 * 86400, 1896134400, false},
		{"legacy zero", `{"config":{"monthlyLimit":{"val":2000},"used":{}}}`, grokPercent(0), 0, 0, false},
		{"modern omitted does not fall back", `{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY"},"monthlyLimit":{"val":2000},"used":{"val":500}}}`, nil, 604800, 0, false},
		{"no config", `{"config":null}`, nil, 0, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != "https://cli-chat-proxy.grok.com/v1/billing?format=credits" || r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("X-XAI-Token-Auth") != "xai-grok-cli" || r.Header.Get("x-grok-client-mode") != "cli" || r.Header.Get("x-grok-client-version") == "" {
					t.Error("wrong Grok billing request")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			defer client.Close()
			usage, err := client.Usage(context.Background(), accounts.Credential{Provider: "xai", AccountID: "synthetic-subject", AccessToken: "synthetic-access"})
			if err != nil || calls != 1 {
				t.Fatal("usage fetch failed", err)
			}
			if test.empty {
				if len(usage.Limits) != 0 {
					t.Fatal("fabricated missing allowance")
				}
				return
			}
			if len(usage.Limits) != 1 || len(usage.Limits[0].Windows) != 1 || usage.ResetCredits != nil {
				t.Fatal("unexpected quota shape", usage)
			}
			window := usage.Limits[0].Windows[0]
			if test.percent == nil {
				if window.UsedPercent != nil {
					t.Fatal("unknown usage became known")
				}
			} else if window.UsedPercent == nil || *window.UsedPercent != *test.percent {
				t.Fatal("wrong included usage", window)
			}
			if test.seconds > 0 && (window.WindowSeconds == nil || *window.WindowSeconds != test.seconds) {
				t.Fatal("wrong period length", window)
			}
			if test.reset > 0 && (window.ResetAt == nil || *window.ResetAt != test.reset) {
				t.Fatal("wrong reset", window)
			}
		})
	}
}
func grokPercent(value float64) *float64 { return &value }

func TestGrokSubscriptionUsageRejectsMalformedMetadata(t *testing.T) {
	for _, body := range []string{`{}`, `{"config":{"creditUsagePercent":-1}}`, `{"config":{"creditUsagePercent":"unknown"}}`, `{"config":{"currentPeriod":{"start":"2030-02-01T00:00:00Z","end":"2030-01-01T00:00:00Z"}}}`, `{"config":{"currentPeriod":{"end":"invalid"}}}`, `{"config":{"monthlyLimit":{"val":2000},"used":{"val":-1}}}`} {
		client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}))
		defer client.Close()
		if _, err := client.Usage(context.Background(), accounts.Credential{Provider: "xai"}); !errors.Is(err, ErrResponse) {
			t.Fatal("malformed metadata accepted", err)
		}
	}
}
