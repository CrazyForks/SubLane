package gateway

import (
	"context"
	"net/http"
	"testing"

	"github.com/murongg/SubLane/internal/groups"
)

func TestSetupDistinguishesPoolFromAccountAndMemberAccess(t *testing.T) {
	ctx := context.Background()
	s, ids := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("setup must not call upstream"); return nil, nil }))
	member := runtimeMember(t, s)
	check := func(user int64, admin bool, key bool, want string) {
		t.Helper()
		got, err := s.SetupProgress(ctx, user, admin, key)
		if err != nil || got.Stage != want {
			t.Fatalf("setup = %+v, %v; want %s", got, err, want)
		}
	}
	check(1, true, false, "key")
	check(member, false, true, "client")
	if _, err := groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	check(1, true, false, "pool")
	check(member, false, false, "access")
	if _, err := s.accounts.SetEnabled(ctx, ids["codex"], false); err != nil {
		t.Fatal(err)
	}
	check(1, true, false, "account")
	if _, err := s.SetupProgress(ctx, 99999, true, true); err == nil {
		t.Fatal("missing membership admitted")
	}
}

func TestSetupCompletionIsPersonalDurableAndOnlySuccessful(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	member := runtimeMember(t, s)
	result, err := s.Open(ctx, member, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	if _, err := s.db.Exec("DELETE FROM request_records; DELETE FROM usage_daily; DELETE FROM usage_hourly"); err != nil {
		t.Fatal(err)
	}
	got, err := s.SetupProgress(ctx, member, false, true)
	if err != nil || got.Stage != "complete" || !got.HasSuccessfulRequest {
		t.Fatal(got, err)
	}
	owner, err := s.SetupProgress(ctx, 1, true, true)
	if err != nil || owner.HasSuccessfulRequest {
		t.Fatal("another member completed owner setup", owner, err)
	}
	if _, err := s.db.Exec("UPDATE memberships SET enabled=0 WHERE user_id=?", member); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetupProgress(ctx, member, false, true); err == nil {
		t.Fatal("disabled membership admitted")
	}
}
