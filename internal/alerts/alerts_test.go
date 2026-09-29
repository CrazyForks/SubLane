package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/tenants"
	"github.com/murongg/SubLane/internal/vault"
)

type senderFunc func(*http.Request) (*http.Response, error)

func (f senderFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func fixture(t *testing.T, sender senderFunc) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	a, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.New(conn, v).Authorize(ctx, "Synthetic account", accounts.Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-subject", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec("UPDATE accounts SET status='reauth_required' WHERE id=?", account.ID); err != nil {
		t.Fatal(err)
	}
	s := New(conn, v, sender)
	s.now = func() time.Time { return time.Unix(1900000000, 0) }
	return s, account.ID
}

func TestAlertsEncryptDeduplicateSurviveRestartAndRecover(t *testing.T) {
	ctx := context.Background()
	var events []Event
	send := senderFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://hooks.example.test/incoming?token=synthetic-secret" {
			t.Fatal("wrong endpoint")
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "synthetic-access") || strings.Contains(string(body), "synthetic-secret") {
			t.Fatal("secret in event")
		}
		var e Event
		if err := json.Unmarshal(body, &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	s, id := fixture(t, send)
	state, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/incoming?token=synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), "synthetic-secret") {
		t.Fatal("secret in settings")
	}
	var stored string
	if err := s.conn.QueryRow("SELECT alert_config FROM tenants WHERE id=1").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "synthetic-secret") {
		t.Fatal("plaintext webhook persisted")
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.conn, s.vault, send)
	restarted.now = s.now
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Status != "firing" || events[0].Kind != "account_reauthorization" {
		t.Fatal(events)
	}
	if _, err := s.conn.Exec("UPDATE accounts SET status='ready' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Status != "resolved" {
		t.Fatal(events)
	}
}

func TestFailedDeliveryRetriesAndDisabledDestinationStopsDelivery(t *testing.T) {
	ctx := context.Background()
	calls := 0
	s, _ := fixture(t, senderFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("synthetic-secret must never appear in status")
	}))
	if _, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/events"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("unbounded retries", calls)
	}
	state, err := s.State(ctx, 1)
	if err != nil || !state.DeliveryFailed || state.NextRetryAt <= 1900000000 {
		t.Fatal(state, err)
	}
	s.now = func() time.Time { return time.Unix(1900003600, 0) }
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	if _, err := s.Update(ctx, 1, Input{Enabled: false, Clear: true}); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1900007200, 0) }
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("disabled webhook delivered")
	}
}

func TestWebhookRejectsLocalOrAmbiguousDestinations(t *testing.T) {
	if _, err := destination("https://" + strings.Repeat("a", 300) + ".example.test/events"); err == nil {
		t.Fatal("overlong host would break bounded settings metadata")
	}
	for _, value := range []string{"http://hooks.example.test", "https://localhost/hook", "https://127.0.0.1", "https://[::1]", "https://169.254.169.254/latest", "https://10.0.0.1", "https://user:pass@example.test", "https://example.test/#fragment", "https://[::ffff:127.0.0.1]"} {
		if _, err := destination(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestPoolAlertIgnoresIsolatedFailuresUntilAnAccountEntersCooldown(t *testing.T) {
	ctx := context.Background()
	s, id := fixture(t, senderFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected delivery"); return nil, nil }))
	if _, err := s.conn.Exec("UPDATE accounts SET status='ready' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err := groups.New(s.conn).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{id}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.conn.Exec("INSERT INTO account_runtime(account_id,failures,cooldown_until) VALUES(?,1,0) ON CONFLICT(account_id) DO UPDATE SET failures=1,cooldown_until=0", id); err != nil {
		t.Fatal(err)
	}
	if err := s.observe(ctx, 1); err != nil {
		t.Fatal(err)
	}
	state, err := s.State(ctx, 1)
	if err != nil || len(state.Incidents) != 0 {
		t.Fatal("isolated failure reported unavailable", state, err)
	}
	if _, err := s.conn.Exec("UPDATE account_runtime SET cooldown_until=1900000300 WHERE account_id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := s.observe(ctx, 1); err != nil {
		t.Fatal(err)
	}
	state, err = s.State(ctx, 1)
	if err != nil || len(state.Incidents) != 1 || state.Incidents[0].Kind != "pool_unavailable" {
		t.Fatal(state, err)
	}
}

func TestAlertsStayWithinWorkspaceAndExcludeCallerRejections(t *testing.T) {
	ctx := context.Background()
	var events []Event
	s, id := fixture(t, senderFunc(func(r *http.Request) (*http.Response, error) {
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	second, err := tenants.New(s.conn).Create(ctx, 1, "Other synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, second.ID, Input{Enabled: true, URL: "https://hooks.example.test/other"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("cross-workspace account notification", events)
	}
	if _, err := s.conn.Exec("UPDATE accounts SET status='ready' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.conn).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/first"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := s.conn.Exec("INSERT INTO request_records(user_id,key_id,group_id,transport,operation,started_at,duration_ms,outcome) VALUES(1,1,?,'http','responses',1900000000,1,'rejected')", pool.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("caller rejection triggered upstream failure alert", events)
	}
	// Long generations belong to the window in which they finish, not the one in which they started.
	if _, err := s.conn.Exec("UPDATE request_records SET outcome='error',started_at=1899999400,duration_ms=600000"); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].WorkspaceID != 1 || events[0].Kind != "request_failures" {
		t.Fatal(events)
	}
	s.now = func() time.Time { return time.Unix(1900000400, 0) }
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Status != "resolved" {
		t.Fatal(events)
	}
}

func TestDisablingAlertsCancelsDeliveryWithoutHoldingDatabase(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	s, _ := fixture(t, senderFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	if _, err := s.Update(ctx, 1, Input{Enabled: true, URL: "https://hooks.example.test/events"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Check(ctx) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("delivery did not start")
	}
	updateCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := s.Update(updateCtx, 1, Input{Clear: true}); err != nil {
		t.Fatal("delivery held settings or database lock", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("delivery was not canceled")
	}
	state, err := s.State(ctx, 1)
	if err != nil || state.Configured || state.DeliveryFailed || len(state.Incidents) != 0 {
		t.Fatal(state, err)
	}
}
