package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
)

func TestRequestHistoryEstimatesCostFromReportedTokens(t *testing.T) {
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("history must not contact upstream")
		return nil, nil
	}))
	s.pricing = pricing.NewStatic(map[string]pricing.Price{
		"synthetic-model": {Input: 2_000_000, Cached: 500_000, Output: 8_000_000},
		"synthetic-flat":  {Input: 2_000_000, Cached: 2_000_000, Output: 8_000_000},
	})
	n := func(value int64) *int64 { return &value }
	for _, tc := range []struct {
		name, model           string
		input, output, cached *int64
		want                  any
	}{
		{"cached input", "codex/synthetic-model", n(1250), n(50), n(1070), float64(1295)},
		{"uncached input", "synthetic-model", n(1250), n(50), n(0), float64(2900)},
		{"small charge", "synthetic-model", n(1), n(0), n(1), float64(1)},
		{"reported zero", "synthetic-model", n(0), n(0), n(0), float64(0)},
		{"missing input", "synthetic-model", nil, n(50), n(0), nil},
		{"missing output", "synthetic-model", n(1250), nil, n(0), nil},
		{"unknown cache discount", "synthetic-model", n(1250), n(50), nil, nil},
		{"cache irrelevant at zero input", "synthetic-model", n(0), n(50), nil, float64(400)},
		{"cache irrelevant at equal rates", "synthetic-flat", n(1250), n(50), nil, float64(2900)},
		{"unpriced model", "synthetic-unpriced", n(1250), n(50), n(0), nil},
		{"inconsistent cache", "synthetic-model", n(10), n(5), n(11), nil},
		{"invalid input", "synthetic-model", n(-1), n(5), n(0), nil},
		{"unbounded usage", "synthetic-model", n(1_000_000_001), n(5), n(0), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if err := s.queries.RecordRequest(ctx, db.RecordRequestParams{
				UserID: 1, GroupID: 1, Model: tc.model, Provider: "codex",
				Transport: "http", Operation: "responses", StartedAt: s.now().Unix(), Outcome: "success",
				InputTokens: tc.input, OutputTokens: tc.output, CachedTokens: tc.cached,
			}); err != nil {
				t.Fatal(err)
			}
			for _, userID := range []int64{0, 1} {
				page, err := s.requests(ctx, userID, RequestFilter{})
				if err != nil || len(page.Requests) == 0 {
					t.Fatal("missing request history", err)
				}
				raw, err := json.Marshal(page.Requests[0])
				if err != nil {
					t.Fatal(err)
				}
				var record map[string]any
				if err := json.Unmarshal(raw, &record); err != nil {
					t.Fatal(err)
				}
				if got, exists := record["estimated_cost_micro_usd"]; !exists || got != tc.want {
					t.Errorf("scope user=%d: estimated cost=%v (present=%v), want %v", userID, got, exists, tc.want)
				}
			}
		})
	}
	s.pricing = nil
	page, err := s.Requests(context.Background(), RequestFilter{Model: "synthetic-flat"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.Requests[0])
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil || record["estimated_cost_micro_usd"] != nil {
		t.Fatal("missing catalog must leave cost unknown", string(raw), err)
	}
}
