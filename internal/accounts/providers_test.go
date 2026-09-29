package accounts

import (
	"context"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderImportsIsolateIdentityAndDiscardEndpointSettings(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
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
	for _, provider := range []string{"claude", "antigravity", "xai"} {
		c, err := ParseFor(provider, []byte(`{"type":"`+provider+`","access_token":"synthetic-access","refresh_token":"synthetic-refresh","email":"member@example.test","expired":"2030-01-01T00:00:00Z","base_url":"https://untrusted.example.test","proxy_url":"http://untrusted.example.test","project_id":"synthetic-project"}`))
		if err != nil {
			t.Fatal(provider, err)
		}
		if c.Kind() != provider || c.AccountID != "member@example.test" {
			t.Fatal("provider identity lost", c)
		}
		if _, ok := c.Metadata["base_url"]; ok {
			t.Fatal("imported endpoint override")
		}
		row, err := service.Authorize(ctx, provider, c, "")
		if err != nil {
			t.Fatal(err)
		}
		if row.Provider != provider {
			t.Fatal("metadata provider lost")
		}
		loaded, err := service.Prepare(ctx, row.ID, nil)
		if err != nil || loaded.Kind() != provider {
			t.Fatal("encrypted provider credential lost", err)
		}
	}
	rows, err := service.List(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatal("same subject across providers collided", err)
	}
}

func TestProviderImportRejectsMismatchedAndKeyCredentials(t *testing.T) {
	for _, raw := range []string{`{"type":"codex","access_token":"synthetic","refresh_token":"synthetic","email":"member@example.test"}`, `{"type":"claude","api_key":"synthetic","access_token":"synthetic","refresh_token":"synthetic","email":"member@example.test"}`} {
		if _, err := ParseFor("claude", []byte(raw)); err == nil {
			t.Fatal("accepted incompatible credential")
		}
	}
}

func TestFailedProviderRefreshCannotPublishUncommittedCredential(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedInitialTenant(t, db)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := New(db, cipher)
	for _, provider := range []string{"codex", "claude", "antigravity", "xai"} {
		original := Credential{Provider: provider, AccessToken: "synthetic-old", RefreshToken: "synthetic-refresh", AccountID: provider, ExpiresAt: 1}
		row, err := service.Authorize(ctx, provider, original, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("CREATE TRIGGER reject_credential_write BEFORE UPDATE OF credential ON accounts BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END"); err != nil {
			t.Fatal(err)
		}
		value, err := service.Prepare(ctx, row.ID, func(context.Context, Credential) (Credential, error) {
			fresh := original
			fresh.AccessToken = "synthetic-new"
			fresh.ExpiresAt = time.Now().Add(time.Hour).Unix()
			return fresh, nil
		})
		if err == nil || value.AccessToken != "" {
			t.Fatal("uncommitted token became usable", provider, err)
		}
		stored, err := service.get(ctx, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		decrypted, err := service.decrypt(stored)
		if err != nil || decrypted.AccessToken != "synthetic-old" {
			t.Fatal("saved credential was replaced", err)
		}
		if _, err = db.Exec("DROP TRIGGER reject_credential_write"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProviderImportValidatesMetadata(t *testing.T) {
	for _, extra := range []string{`"provider":"codex"`, `"project_id":{}`, `"organization_name":12`, `"claude_device_ids":[{}]`, `"project_id":"` + strings.Repeat("x", 1025) + `"`} {
		if _, err := ParseFor("claude", []byte(`{"type":"claude","access_token":"synthetic","refresh_token":"synthetic","email":"member@example.test",`+extra+`}`)); err == nil {
			t.Fatal("invalid metadata accepted", extra)
		}
	}
}

func TestGrokImportUsesSubjectAndRejectsIdentityMismatch(t *testing.T) {
	raw := []byte(`{"type":"xai","access_token":"synthetic-access","refresh_token":"synthetic-refresh","sub":"synthetic-subject","email":"member@example.test","expired":"2030-01-01T00:00:00Z","auth_kind":"oauth","base_url":"https://untrusted.example.test","using_api":true}`)
	c, err := ParseFor("xai", raw)
	if err != nil || c.AccountID != "synthetic-subject" {
		t.Fatal("Grok subject not preserved", err)
	}
	if c.Metadata["base_url"] != nil || c.Metadata["using_api"] != nil {
		t.Fatal("imported execution overrides")
	}
	for _, extra := range []string{`"auth_kind":"apikey"`, `"account_id":"other-subject","sub":"synthetic-subject"`} {
		if _, err := ParseFor("xai", []byte(`{"type":"xai","access_token":"synthetic-access","refresh_token":"synthetic-refresh","email":"member@example.test",`+extra+`}`)); err == nil {
			t.Fatal("invalid Grok credential accepted")
		}
	}
}
