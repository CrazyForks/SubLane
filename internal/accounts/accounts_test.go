package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func seedInitialTenant(t *testing.T, connection *sql.DB) {
	t.Helper()
	if _, err := connection.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at)
		VALUES(1,'synthetic-admin','admin','synthetic-hash',1,1);
		INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1);
		INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES(1,1,'owner',1)`); err != nil {
		t.Fatal(err)
	}
}

func TestAccountsAreScopedToWorkspace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	if _, err := connection.Exec(`INSERT INTO users(id,username,role,password_hash,enabled,created_at)
		VALUES(2,'synthetic-second','member','synthetic-hash',1,1);
		INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(2,'Second workspace',2,1);
		INSERT INTO memberships(tenant_id,user_id,role,created_at) VALUES(2,2,'owner',1)`); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	first := New(connection, cipher)
	second := NewForTenant(connection, cipher, 2)
	credential := Credential{AccountID: "shared-upstream-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	a, err := first.Authorize(ctx, "First account", credential, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Authorize(ctx, "Second account", credential, "")
	if err != nil {
		t.Fatalf("same upstream subject in another workspace was rejected: %v", err)
	}
	for _, check := range []struct {
		service *Service
		own     string
		other   string
	}{{first, a.ID, b.ID}, {second, b.ID, a.ID}} {
		values, err := check.service.List(ctx)
		if err != nil || len(values) != 1 || values[0].ID != check.own {
			t.Fatalf("workspace account list leaked: %+v, %v", values, err)
		}
		if _, err := check.service.Get(ctx, check.other); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign workspace account was readable: %v", err)
		}
	}
	if err := second.Delete(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace account was deleted: %v", err)
	}
	if _, err := first.Get(ctx, a.ID); err != nil {
		t.Fatalf("foreign delete removed the owner's account: %v", err)
	}
	if _, err := second.Catalog(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace model catalog was readable: %v", err)
	}
}

func TestAccountCredentialsAndLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(connection, cipher)
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"account-test"}}`)
	account, err := service.Import(ctx, "Test subscription", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if account.ID == "" || !account.Enabled || account.Name != "Test subscription" || account.MaxConcurrency != 30 {
		t.Fatal("invalid account metadata")
	}
	if _, err := service.Import(ctx, "Duplicate", raw, ""); !errors.Is(err, ErrDuplicate) {
		t.Fatal("duplicate identity accepted", err)
	}
	list, err := service.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal("account missing", err)
	}
	if list[0].MaxConcurrency != 30 {
		t.Fatal("new account did not persist the default concurrency", list[0].MaxConcurrency)
	}
	if list[0].GroupCount == nil || *list[0].GroupCount != 0 {
		t.Fatal("new account must be unassigned", list)
	}
	if _, err := connection.Exec("INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1); INSERT INTO group_accounts(group_id,account_id) VALUES(1,?)", account.ID); err != nil {
		t.Fatal(err)
	}
	assigned, err := service.List(ctx)
	if err != nil || assigned[0].GroupCount == nil || *assigned[0].GroupCount != 1 {
		t.Fatal("assignment count not updated", assigned, err)
	}
	payload, _ := json.Marshal(list)
	if strings.Contains(string(payload), "synthetic-access") || strings.Contains(string(payload), "refresh_token") {
		t.Fatal("credentials exposed")
	}
	var encrypted []byte
	if err := connection.QueryRow("SELECT credential FROM accounts WHERE id = ?", account.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), "synthetic-") {
		t.Fatal("plaintext credential persisted")
	}
	wrongKey, err := vault.Open(filepath.Join(dir, "wrong-key"), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := New(connection, wrongKey).Verify(ctx); !errors.Is(err, vault.ErrDecrypt) {
		t.Fatal("replaced encryption key accepted", err)
	}
	refreshes := 0
	refresh := func(ctx context.Context, old Credential) (Credential, error) {
		refreshes++
		if old.RefreshToken != "synthetic-refresh" {
			t.Fatal("incorrect refresh token")
		}
		old.AccessToken = "synthetic-rotated-access"
		old.RefreshToken = "synthetic-rotated-refresh"
		old.ExpiresAt = time.Now().Add(time.Hour).Unix()
		return old, nil
	}
	prepared, err := service.Prepare(ctx, account.ID, refresh)
	if err != nil || prepared.AccessToken != "synthetic-rotated-access" {
		t.Fatal("refresh failed", err)
	}
	if _, err := service.Prepare(ctx, account.ID, refresh); err != nil || refreshes != 1 {
		t.Fatal("refreshed more than once", err)
	}
	rejectedRefreshes := 0
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			current, err := service.RefreshAfterRejection(ctx, account.ID, "synthetic-rotated-access", func(ctx context.Context, old Credential) (Credential, error) {
				rejectedRefreshes++
				old.AccessToken = "synthetic-recovered-access"
				return old, nil
			})
			if err != nil || current.AccessToken != "synthetic-recovered-access" {
				t.Error("concurrent refresh failed", err)
			}
		})
	}
	wait.Wait()
	if rejectedRefreshes != 1 {
		t.Fatal("rejected token refreshed more than once")
	}
	if err := service.RecordUse(ctx, account.ID, "synthetic-rotated-access", false); err != nil {
		t.Fatal(err)
	}
	checked, err := service.Get(ctx, account.ID)
	if err != nil || checked.Status != "ready" {
		t.Fatal("stale rejection invalidated newer credentials")
	}
	if _, err := service.SetEnabled(ctx, account.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare(ctx, account.ID, refresh); !errors.Is(err, ErrDisabled) {
		t.Fatal("disabled account usable", err)
	}
	if _, err := service.SetEnabled(ctx, account.ID, true); err != nil {
		t.Fatal(err)
	}
	other := []byte(`{"access_token":"other-access","refresh_token":"other-refresh","account_id":"other-account"}`)
	if _, err := service.Import(ctx, "Wrong identity", other, account.ID); !errors.Is(err, ErrIdentity) {
		t.Fatal("reauthorization changed identity", err)
	}
	if err := service.Delete(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare(ctx, account.ID, refresh); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted account usable", err)
	}
}

func TestRejectsOfficialAPIKeysAndInvalidImports(t *testing.T) {
	for _, raw := range []string{
		`{"OPENAI_API_KEY":"synthetic-official-key"}`,
		`{"auth_mode":"apikey","tokens":{"access_token":"a","refresh_token":"r","account_id":"fake-id"}}`,
		`{"access_token":"token"}`,
		`null`, `[]`, `{`,
	} {
		if _, err := ParseCredential([]byte(raw)); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	credential, err := ParseCredential([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"account-test","base_url":"https://other.example.test","proxy_url":"https://other.example.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(credential)
	if strings.Contains(string(data), "other.example.test") {
		t.Fatal("untrusted endpoint imported")
	}
}
