package upstream

import (
	"context"
	"github.com/murongg/SubLane/internal/accounts"
	"testing"
)

func TestEngineBootstrapsExecutorsWithoutCredentialMirror(t *testing.T) {
	client := New()
	defer client.Close()
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"codex", "claude", "antigravity", "xai"} {
		if _, err := client.executor(provider); err != nil {
			t.Fatal(provider, err)
		}
	}
	if len(client.runtime.manager.List()) != 0 {
		t.Fatal("SDK must not become a second credential store")
	}
}

func TestExecutionAuthCannotRefreshOutsideDurableOwner(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "antigravity", "xai"} {
		auth := sdkAuth(accounts.Credential{Provider: provider, AccountID: "synthetic-account", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh"})
		if auth.Metadata["refresh_token"] != nil {
			t.Fatal("executor received a refresh token outside the durable owner", provider)
		}
		if auth.Metadata["access_token"] != "synthetic-access" {
			t.Fatal("execution credential missing")
		}
	}
}
