package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/alerts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestVaultReopensInitializedWorkspace(t *testing.T) {
	for _, webhook := range []bool{false, true} {
		name := "without webhook"
		if webhook {
			name = "with encrypted webhook"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			path := filepath.Join(dir, "sublane.db")
			connection, err := storage.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			identity, err := auth.New(connection)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := identity.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic workspace"); err != nil {
				t.Fatal(err)
			}
			if webhook {
				cipher, err := vault.Open(filepath.Join(dir, "credentials.key"), true)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := alerts.New(connection, cipher, nil).Update(ctx, 1, alerts.Input{URL: "https://hooks.example.test/synthetic"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := connection.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := storage.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			cipher, err := openVault(ctx, reopened, dir)
			if err != nil {
				t.Fatalf("initialized workspace failed to reopen: %v", err)
			}
			if err := alerts.New(reopened, cipher, nil).Verify(ctx); err != nil {
				t.Fatalf("persisted webhook cannot be decrypted: %v", err)
			}
		})
	}
}

func TestMissingVaultKeyIsNotRecreatedForWebhookOnlyInstance(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic"); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "credentials.key")
	v, err := vault.Open(key, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alerts.New(conn, v, nil).Update(ctx, 1, alerts.Input{URL: "https://hooks.example.test/events"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err := openVault(ctx, conn, dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing webhook encryption key, got %v", err)
	}
}

func TestMissingVaultKeyIsNotRecreatedForGatewaySecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "sublane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-pass", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.key")
	cipher, err := vault.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec("INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := apikey.New(connection, cipher).CreateInGroup(ctx, 1, 1, "Synthetic key"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := openVault(ctx, connection, dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing gateway encryption key, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("new encryption file was created")
	}
}
