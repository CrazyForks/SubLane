package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionlessRequestsRotateWithoutPersistingAffinity(t *testing.T) {
	ctx := context.Background()
	service, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return nil, nil }))
	_, err := service.accounts.Authorize(ctx, "Second", accounts.Credential{AccountID: "synthetic-second", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	configureTestPool(t, service.db)
	first, one, err := service.selectAccount(ctx, 1, 1, "", "codex", "", Responses)
	if err != nil {
		t.Fatal(err)
	}
	second, two, err := service.selectAccount(ctx, 1, 1, "", "codex", "", Responses)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || one == two {
		t.Fatal("sessionless requests reused an account binding/correlation")
	}
	count, err := service.queries.CountAccountAffinity(ctx)
	if err != nil || count != 0 {
		t.Fatal("sessionless affinity persisted", count, err)
	}
}

func syntheticStream() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"))}
}

func TestAccountConcurrencySupportsThirtyAndPreservesLimitAcrossRestart(t *testing.T) {
	ctx := context.Background()
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	for _, limit := range []int64{1, 30} {
		if err := s.SetConcurrency(ctx, ids["codex"], limit); err != nil {
			t.Fatal("valid account limit rejected", limit, err)
		}
	}
	for _, limit := range []int64{0, 31} {
		if err := s.SetConcurrency(ctx, ids["codex"], limit); !errors.Is(err, accounts.ErrInput) {
			t.Fatal("invalid account limit accepted", limit, err)
		}
	}
	restarted := New(ctx, s.db, s.accounts, s.provider)
	defer restarted.Close()
	states, err := restarted.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, state := range states {
		if state.ID == ids["codex"] {
			found = true
			if state.MaxConcurrency != 30 {
				t.Fatal("restart lost account concurrency", state.MaxConcurrency)
			}
		}
	}
	if !found {
		t.Fatal("restart lost the account runtime")
	}
}

func TestAccountLeaseCoversStreamAndRejectsStickyOverload(t *testing.T) {
	ctx := context.Background()
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	if err := service.SetConcurrency(ctx, ids["codex"], 1); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"model":"synthetic-model","input":"synthetic prompt"}`)
	headers := http.Header{"Session_id": {"synthetic-session"}}
	first, err := service.Open(ctx, 1, 1, raw, headers, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(ctx, 1, 1, raw, headers, Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("sticky request exceeded account limit", err)
	}
	shared, err := groups.New(service.db).Save(ctx, 0, groups.Input{Name: "Shared", Enabled: true, AccountIDs: []string{ids["codex"]}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(ctx, 1, shared.ID, raw, nil, Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("another group bypassed account concurrency", err)
	}

	if err := first.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	first.Body.Close()
	again, err := service.Open(ctx, 1, 1, raw, headers, Responses)
	if err != nil {
		t.Fatal("lease was not released", err)
	}
	again.Body.Close()
	history, err := service.Requests(ctx, RequestFilter{})
	if err != nil || len(history.Requests) != 4 {
		t.Fatal("missing bounded history", err)
	}
	for _, r := range history.Requests {
		if r.Outcome == "success" && (r.InputTokens == nil || *r.InputTokens != 3) {
			t.Fatal("reported usage not captured")
		}
	}
}

func TestRateLimitCooldownSurvivesRestartAndOnlyOneRecoveryProbe(t *testing.T) {
	ctx := context.Background()
	var rejected atomic.Bool
	rejected.Store(true)
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		if rejected.Load() {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return syntheticStream(), nil
	}))
	clock := atomic.Int64{}
	clock.Store(time.Now().UnixMilli())
	service.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	raw := []byte(`{"model":"synthetic-model","input":"synthetic"}`)
	response, err := service.Open(ctx, 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	restarted := New(ctx, service.db, service.accounts, service.provider)
	defer restarted.Close()
	restarted.now = service.now
	if _, err := restarted.Open(ctx, 1, 1, raw, nil, Responses); !errors.Is(err, ErrAccountCooling) {
		t.Fatal("restart lost cooldown", err)
	}
	clock.Add(61_000)
	rejected.Store(false)
	probe, err := restarted.Open(ctx, 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Open(ctx, 1, 1, raw, nil, Responses); !errors.Is(err, ErrAccountBusy) {
		t.Fatal("multiple recovery probes admitted", err)
	}
	if err := probe.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	probe.Body.Close()
	states, err := restarted.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && state.Failures != 0 {
			t.Fatal("successful probe did not recover")
		}
	}
}

func TestTransientFailuresCooldownAndResumeIgnoreOlderRequests(t *testing.T) {
	ctx := context.Background()
	var failing atomic.Bool
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		if failing.Load() {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"private":"synthetic-body"}`))}, nil
		}
		return syntheticStream(), nil
	}))
	raw := []byte(`{"model":"synthetic-model","input":"synthetic-private-prompt"}`)
	older, err := service.Open(ctx, 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	failing.Store(true)
	for range 3 {
		r, err := service.Open(ctx, 1, 1, raw, nil, Responses)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
	}
	if err := older.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	older.Body.Close()
	if _, err := service.Open(ctx, 1, 1, raw, nil, Responses); !errors.Is(err, ErrAccountCooling) {
		t.Fatal("older success cleared newer failures", err)
	}
	if err := service.Resume(ctx, ids["codex"]); err != nil {
		t.Fatal(err)
	}
	failing.Store(false)
	r, err := service.Open(ctx, 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	page, err := service.Requests(ctx, RequestFilter{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "synthetic-private-prompt") || strings.Contains(string(encoded), "synthetic-body") {
		t.Fatal("private content leaked into request records")
	}
}

func TestCancellationAndShutdownReleaseAccountLease(t *testing.T) {
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	ctx, cancel := context.WithCancel(context.Background())
	r, err := service.Open(ctx, 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	r.Body.Close()
	states, err := service.Runtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && (state.InFlight != 0 || state.Failures != 0) {
			t.Fatal("cancellation penalized or leaked account", state)
		}
	}
	_, err = service.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { service.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not drain leases")
	}
}

func TestRetryAfterAndHistoryRetention(t *testing.T) {
	now := time.Now()
	if retryDelay("999999", now) != 3600 || retryDelay("invalid", now) != 60 || retryDelay(now.Add(2*time.Minute).UTC().Format(http.TimeFormat), now) < 119 {
		t.Fatal("retry-after bounds/date ignored")
	}
	service, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return nil, nil }))
	tx, err := service.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	q := service.queries.WithTx(tx)
	for i := 0; i < 5003; i++ {
		started := now.Unix()
		if i == 5002 {
			started = now.Add(-8 * 24 * time.Hour).Unix()
		}
		if err := q.RecordRequest(context.Background(), db.RecordRequestParams{UserID: 1, GroupID: 1, Transport: "http", Operation: "responses", Outcome: "success", StartedAt: started}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err := service.Requests(context.Background(), RequestFilter{})
	if err != nil || len(page.Requests) != 50 || page.NextCursor == 0 {
		t.Fatal("history page invalid", err)
	}
	var count int
	if err := service.db.QueryRow("SELECT count(*) FROM request_records").Scan(&count); err != nil || count != 5003 {
		t.Fatal("history read changed stored records", count, err)
	}
	if err := pruneHistory(context.Background(), service.db, now); err != nil {
		t.Fatal(err)
	}
	if err := service.db.QueryRow("SELECT count(*) FROM request_records").Scan(&count); err != nil || count > 5000 {
		t.Fatal("maintenance did not bound history", count, err)
	}
	previous := page.Requests[0].ID
	if _, err := service.db.Exec("UPDATE request_records SET started_at=1"); err != nil {
		t.Fatal(err)
	}
	if err := pruneHistory(context.Background(), service.db, now); err != nil {
		t.Fatal(err)
	}
	if err := service.queries.RecordRequest(context.Background(), db.RecordRequestParams{UserID: 1, GroupID: 1, Transport: "http", Operation: "responses", Outcome: "success", StartedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	page, err = service.Requests(context.Background(), RequestFilter{})
	if err != nil || page.Requests[0].ID <= previous {
		t.Fatal("history cursor reused after retention cleanup", err)
	}

}

func TestLateSuccessCannotEraseFailureAtSameTimestamp(t *testing.T) {
	var fail atomic.Bool
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		if fail.Load() {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return syntheticStream(), nil
	}))
	fixed := time.Now()
	service.now = func() time.Time { return fixed }
	raw := []byte(`{"model":"synthetic-model","input":"synthetic"}`)
	old, err := service.Open(context.Background(), 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	bad, err := service.Open(context.Background(), 1, 1, raw, nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if err := old.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	old.Body.Close()
	states, err := service.Runtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && state.Failures != 1 {
			t.Fatal("late success erased new failure")
		}
	}
}

func TestDownstreamContextLimitDoesNotPenalizeAccount(t *testing.T) {
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	r, err := service.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Events(func([]byte) error { return ErrContextLimit }); !errors.Is(err, ErrContextLimit) {
		t.Fatal(err)
	}
	r.Body.Close()
	page, err := service.Requests(context.Background(), RequestFilter{})
	if err != nil || page.Requests[0].Outcome != "rejected" || page.Requests[0].ErrorCode != "context_limit" {
		t.Fatal("local context rejection mislabeled", err)
	}
	states, err := service.Runtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && state.Failures != 0 {
			t.Fatal("local context limit penalized provider")
		}
	}
}
func TestManualResumeIgnoresPreviouslyStartedFailures(t *testing.T) {
	service, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	r, err := service.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Resume(context.Background(), ids["codex"]); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	states, err := service.Runtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.ID == ids["codex"] && (state.Failures != 0 || state.InFlight != 0 || state.CooldownUntil != 0) {
			t.Fatal("old result reinstated cooldown", state)
		}
	}
}
