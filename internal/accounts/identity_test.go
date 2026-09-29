package accounts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClaudeIdentitySurvivesRefreshReauthorizationAndRestart(t *testing.T) {
	s, account, now := refreshFixture(t, "claude", time.Minute)
	ctx := context.Background()
	device := savedClaudeDevice(t, s, account.ID)
	updated, err := s.Prepare(ctx, account.ID, func(_ context.Context, c Credential) (Credential, error) {
		if savedClaudeDevice(t, s, account.ID) != device {
			t.Error("identity was not saved before provider IO")
		}
		c.AccessToken = "synthetic-rotated"
		c.ExpiresAt = now.Add(time.Hour).Unix()
		c.Metadata = map[string]json.RawMessage{"claude_device_ids": json.RawMessage(`["` + strings.Repeat("b", 64) + `"]`)}
		return c, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if savedClaudeDevice(t, s, account.ID) != device {
		t.Fatal("token rotation replaced the account device")
	}
	updated.Metadata = nil
	if _, err := s.Authorize(ctx, "Synthetic reauthorization", updated, account.ID); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.db, s.vault)
	restarted.now = s.now
	prepared, err := restarted.Prepare(ctx, account.ID, nil)
	if err != nil || prepared.AccessToken != "synthetic-rotated" || savedClaudeDevice(t, restarted, account.ID) != device {
		t.Fatalf("reauthorization or restart lost identity: %v", err)
	}
}

func TestClaudeImportedIdentityIsRetained(t *testing.T) {
	s, _, now := refreshFixture(t, "codex", time.Hour)
	device := strings.Repeat("a", 64)
	c := Credential{Provider: "claude", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-import", ExpiresAt: now.Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"claude_device_ids": json.RawMessage(`["` + device + `"]`)}}
	account, err := s.Authorize(context.Background(), "Synthetic import", c, "")
	if err != nil || savedClaudeDevice(t, s, account.ID) != device {
		t.Fatalf("imported device identity was replaced: %v", err)
	}
}

func TestLegacyClaudeIdentityCannotEscapeFailedPersistence(t *testing.T) {
	s, account, _ := refreshFixture(t, "claude", time.Hour)
	ctx := context.Background()
	row, err := s.get(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.decrypt(row)
	if err != nil {
		t.Fatal(err)
	}
	delete(c.Metadata, "claude_device_ids")
	if err := s.persist(ctx, s.queries, account.ID, c, row.Status); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_identity_write BEFORE UPDATE OF credential ON accounts BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END"); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.Prepare(ctx, account.ID, nil)
	if err == nil || prepared.AccessToken != "" {
		t.Fatal("legacy account became executable before its identity was saved")
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_identity_write"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(ctx, account.ID, nil); err != nil {
		t.Fatal(err)
	}
	device := savedClaudeDevice(t, s, account.ID)
	restarted := New(s.db, s.vault)
	restarted.now = s.now
	if _, err := restarted.Prepare(ctx, account.ID, nil); err != nil || savedClaudeDevice(t, restarted, account.ID) != device {
		t.Fatalf("legacy identity did not survive restart: %v", err)
	}
}

func savedClaudeDevice(t *testing.T, s *Service, id string) string {
	t.Helper()
	row, err := s.get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.decrypt(row)
	if err != nil {
		t.Fatal(err)
	}
	var devices []string
	if json.Unmarshal(c.Metadata["claude_device_ids"], &devices) != nil || len(devices) != 1 {
		t.Fatal("credential lacks a persisted Claude device")
	}
	raw, err := hex.DecodeString(devices[0])
	if err != nil || len(raw) != 32 {
		t.Fatal("invalid Claude device identity")
	}
	return devices[0]
}
