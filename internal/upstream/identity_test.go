package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestClaudeOAuthExecutionKeepsPersistedIdentity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.Setup(ctx, "owner-test", "synthetic pass 42", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := accounts.New(db, cipher)
	uuid := "00000000-0000-4000-8000-000000000001"
	c := accounts.Credential{Provider: "claude", AccessToken: "sk-ant-oat01-synthetic-test", RefreshToken: "synthetic-refresh", AccountID: uuid, Email: "member@example.test", ExpiresAt: time.Now().Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"account_uuid": json.RawMessage(`"` + uuid + `"`)}}
	account, err := service.Authorize(ctx, "Synthetic Claude subscription", c, "")
	if err != nil {
		t.Fatal(err)
	}
	var identities []string
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer "+c.AccessToken {
			return nil, fmt.Errorf("Claude OAuth authentication was not exercised")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		var body struct {
			Metadata struct {
				UserID string `json:"user_id"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		identities = append(identities, body.Metadata.UserID)
		response := `{"id":"msg_synthetic","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"synthetic"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
	}))
	defer client.Close()
	for range 2 {
		prepared, err := service.Prepare(ctx, account.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		stream, err := client.Messages(ctx, prepared, []byte(`{"model":"claude-sonnet-4-5","max_tokens":1,"messages":[{"role":"user","content":"synthetic"}]}`), http.Header{"Session_id": {"synthetic-session"}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, stream.Body)
		stream.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		service = accounts.New(db, cipher)
	}
	if len(identities) != 2 || identities[0] == "" || identities[0] != identities[1] {
		t.Fatal("same account and session changed Claude identity across restart")
	}
}
