package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
)

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
