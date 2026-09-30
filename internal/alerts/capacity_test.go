package alerts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/capacity"
)

func TestTestNotificationIsBoundedAndDoesNotChangeIncidents(t *testing.T) {
	ctx := context.Background()
	calls := 0
	s, _ := fixture(t, senderFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"kind":"test"`) || strings.Contains(string(body), "synthetic-secret") {
			t.Fatal(string(body))
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := s.Test(ctx, 1); !errors.Is(err, ErrInput) {
		t.Fatal("missing destination", err)
	}
	if _, err := s.Update(ctx, 1, Input{URL: "https://hooks.example.test/synthetic-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Test(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Test(ctx, 1); !errors.Is(err, ErrTestCooling) {
		t.Fatal("unbounded testing", err)
	}
	state, err := s.State(ctx, 1)
	if err != nil || len(state.Incidents) != 0 || state.LastDeliveredAt != 0 || calls != 1 {
		t.Fatal(state, calls, err)
	}
}

func TestCapacityAlertsDeduplicateRecoverAndIgnoreUnknownAndBusy(t *testing.T) {
	ctx := context.Background()
	calls := 0
	s, _ := fixture(t, senderFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if _, err := s.conn.Exec("UPDATE accounts SET status='ready'"); err != nil {
		t.Fatal(err)
	}
	used := 95.0
	pools := []capacity.Pool{{ID: 7, Models: []capacity.Model{{Model: "synthetic-model", State: "unavailable", Reasons: []capacity.Reason{{Code: "quota_exhausted", Count: 1}}, Accounts: []capacity.Account{{ID: "synthetic-account", Reason: "quota_exhausted", QuotaState: "exhausted", QuotaUsedPercent: &used}}}}}}
	s.SetCapacityObserver(func(context.Context, int64) ([]capacity.Pool, error) { return pools, nil })
	if _, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/events", QuotaThreshold: 90, ModelAlerts: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := s.State(ctx, 1)
	if err != nil || len(state.Incidents) != 2 || calls != 2 {
		t.Fatal(state, calls, err)
	}
	pools[0].Models[0].State = "unknown"
	pools[0].Models[0].Unknown = 1
	pools[0].Models[0].Accounts[0].QuotaState = "stale"
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("unknown observations sent recovery", calls)
	}
	observed := pools[0].Models
	pools[0].Models = nil
	pools[0].Unknown = true
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("invalidated catalog falsely recovered", calls)
	}
	pools[0].Models = observed
	pools[0].Unknown = false
	pools[0].Models[0].State = "unavailable"
	pools[0].Models[0].Unknown = 0
	pools[0].Models[0].Reasons = []capacity.Reason{{Code: "account_busy", Count: 1}}
	pools[0].Models[0].Accounts[0].QuotaState = "available"
	used = 30
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = s.State(ctx, 1)
	if err != nil || len(state.Incidents) != 0 || calls != 4 {
		t.Fatal(state, calls, err)
	}
}

func TestCapacityObservationFailureDoesNotSuppressExistingAlerts(t *testing.T) {
	calls := 0
	s, _ := fixture(t, senderFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	s.SetCapacityObserver(func(context.Context, int64) ([]capacity.Pool, error) {
		return nil, errors.New("synthetic observation failure")
	})
	if _, err := s.Update(context.Background(), 1, Input{Enabled: true, URL: "https://hooks.example.test/events", ModelAlerts: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("base alert was suppressed", calls)
	}
}
