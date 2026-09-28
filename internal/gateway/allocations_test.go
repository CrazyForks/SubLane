package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/upstream"
)

func TestFailedTerminalUsageSettlesAllocation(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	ids, err := s.queries.ListGroupAccounts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic failed pool", Enabled: true, AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := groups.New(s.db).SetMemberGroups(ctx, user, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	scheme, err := allocations.New(s.db).SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic failed usage", GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "tokens", Period: "day", Members: []allocations.Share{{UserID: user, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.db.ExecContext(ctx, "INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,?,'synthetic','sl_fake',randomblob(32),1)", user, pool.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := result.LastInsertId()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", key, scheme.ID); err != nil {
		t.Fatal(err)
	}
	entry, err := s.begin(WithRequestIdentity(ctx, key, "http"), user, pool.ID, Responses)
	if err != nil {
		t.Fatal(err)
	}
	entry.record.Provider = "codex"
	entry.record.AccountID = ids[0]
	entry.record.Model = "synthetic-model"
	entry.upstreamDispatched = true
	entry.allocationTracked = true
	if err := allocations.Begin(ctx, s.queries, allocations.Request{ID: entry.record.RequestID, SchemeID: scheme.ID, UserID: user, GroupID: pool.ID, AccountID: ids[0], Model: "synthetic-model", StartedAt: entry.started.Unix()}, s.now().Unix(), s.location()); err != nil {
		entry.fail(err)
		t.Fatal(err)
	}
	body := `data: {"type":"response.failed","response":{"usage":{"input_tokens":3,"output_tokens":1}}}` + "\n\n"
	stream := &upstream.Stream{Response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}}
	x := trackExchange(stream, entry, entry.ctx, nil)
	if err := x.Events(func([]byte) error { return nil }); !errors.Is(err, upstream.ErrInterrupted) {
		t.Fatal("failed terminal did not interrupt the request", err)
	}
	x.Body.Close()
	detail, err := s.Allocations().Detail(ctx, scheme.ID, user)
	if err != nil || len(detail.Pending) != 0 || len(detail.Balances) != 1 || detail.Balances[0].Used != 4 {
		t.Fatal("trusted failure usage did not settle", detail, err)
	}
}

func TestFailedAllocationWriteRecoversWithoutChangingOtherActiveRequests(t *testing.T) {
	ctx := context.Background()
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	a, err := auth.New(s.db)
	if err != nil {
		t.Fatal(err)
	}
	createdMember, err := a.CreateMember(ctx, "synthetic-other-member", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	otherUser := createdMember.ID
	if err := s.SetConcurrency(ctx, ids["codex"], 4); err != nil {
		t.Fatal(err)
	}
	accounts, err := s.queries.ListGroupAccounts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic recovery pool", Enabled: true, AccountIDs: accounts})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := groups.New(s.db).SetMemberGroups(ctx, user, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	if err := groups.New(s.db).SetMemberGroups(ctx, otherUser, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	scheme, err := allocations.New(s.db).SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic write recovery", GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "tokens", Period: "day", Members: []allocations.Share{{UserID: user, Limit: 100}, {UserID: otherUser, Limit: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.db.ExecContext(ctx, "INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,?,'synthetic','sl_fake',randomblob(32),1)", user, pool.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := result.LastInsertId()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", key, scheme.ID); err != nil {
		t.Fatal(err)
	}
	otherResult, err := s.db.ExecContext(ctx, "INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,?,'synthetic-other','sl_other',randomblob(32),1)", otherUser, pool.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, _ := otherResult.LastInsertId()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", otherKey, scheme.ID); err != nil {
		t.Fatal(err)
	}
	open := func() *Exchange {
		t.Helper()
		x, err := s.Open(WithRequestIdentity(ctx, key, "http"), user, pool.ID, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	first := open()
	other := open()
	if _, err := s.db.ExecContext(ctx, "CREATE TRIGGER synthetic_write_failure BEFORE INSERT ON request_records BEGIN SELECT RAISE(FAIL, 'synthetic write failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := first.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	otherMember, err := s.Open(WithRequestIdentity(ctx, otherKey, "http"), otherUser, pool.ID, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal("one member's failed settlement blocked another member", err)
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER synthetic_write_failure"); err != nil {
		t.Fatal(err)
	}
	third := open()
	var firstState, otherState string
	if err := s.db.QueryRowContext(ctx, "SELECT state FROM allocation_entries WHERE request_id=?", first.entry.record.RequestID).Scan(&firstState); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT state FROM allocation_entries WHERE request_id=?", other.entry.record.RequestID).Scan(&otherState); err != nil {
		t.Fatal(err)
	}
	if firstState != "settled" || otherState != "active" {
		t.Fatal("recovery changed the wrong allocation entries", firstState, otherState)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM request_records WHERE request_id=?", first.entry.record.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed request was not recorded exactly once", count, err)
	}
	s.mu.Lock()
	s.failedSettlements = append(s.failedSettlements, first.entry)
	s.mu.Unlock()
	if err := s.PrepareAllocations(ctx); err != nil {
		t.Fatal("an already committed settlement could not be retried", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM request_records WHERE request_id=?", first.entry.record.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatal("retry duplicated request accounting", count, err)
	}
	for _, x := range []*Exchange{other, otherMember, third} {
		if err := x.Events(func([]byte) error { return nil }); err != nil {
			t.Fatal(err)
		}
		x.Body.Close()
	}
}

func TestAllocationKeyUsesOneModeAndSharedLedger(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	ids, err := s.queries.ListGroupAccounts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager := allocations.New(s.db)
	if err := groups.New(s.db).SetMemberGroups(ctx, user, []int64{pool.ID}); err != nil {
		t.Fatal(err)
	}
	scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic amount", GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "amount", Period: "day", Members: []allocations.Share{{UserID: user, Limit: 16}}, Rates: []allocations.Rate{{Model: "synthetic-model", Input: 2000000, Cached: 1000000, Output: 6000000}}}})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic stream has 3 input (2 cached) and 2 output = 16 micro-USD.
	// Bind synthetic hashed key rows through the same durable relation as personal keys.
	result, err := s.db.Exec("INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,2,'synthetic','sl_fake',randomblob(32),1)", user)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := result.LastInsertId()
	if _, err = s.db.Exec("INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", key, scheme.ID); err != nil {
		t.Fatal(err)
	}
	callctx := WithRequestIdentity(ctx, key, "http")
	x, err := s.Open(callctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := x.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	x.Body.Close()
	if _, err = s.Open(WithRequestIdentity(ctx, key, "websocket"), user, pool.ID, []byte(`{"model":"codex/synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrQuota) {
		t.Fatal("scheme did not enforce its amount limit", err)
	}
	if _, err = s.Open(ctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrUnavailable) {
		t.Fatal("unbound key bypassed scheme", err)
	}
}

func TestPrepareAllocationsRecoversWithoutLegacyBudgetTables(t *testing.T) {
	ctx := context.Background()
	s, accounts := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	created, err := s.db.ExecContext(ctx, "INSERT INTO allocation_schemes(name,group_id,enabled,created_at) VALUES('Synthetic recovery',1,1,1)")
	if err != nil {
		t.Fatal(err)
	}
	schemeID, err := created.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	created, err = s.db.ExecContext(ctx, "INSERT INTO allocation_revisions(scheme_id,effective_at,config) VALUES(?,1,'{}')", schemeID)
	if err != nil {
		t.Fatal(err)
	}
	revisionID, err := created.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO allocation_entries(request_id,scheme_id,revision_id,user_id,account_id,model,mode,window_start,reset_at,started_at,state)
		VALUES('synthetic-recovery',?,?,1,?,'synthetic-model','tokens',1,2,1,'active')`, schemeID, revisionID, accounts["codex"]); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"token_budget_entries", "token_budget_usage", "token_budgets"} {
		if _, err := s.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PrepareAllocations(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.db.QueryRowContext(ctx, "SELECT state FROM allocation_entries WHERE request_id='synthetic-recovery'").Scan(&state); err != nil || state != "pending" {
		t.Fatalf("interrupted allocation not recovered: state=%q err=%v", state, err)
	}
}
