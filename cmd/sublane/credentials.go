package main

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

func openVault(ctx context.Context, connection *sql.DB, directory string) (*vault.Vault, error) {
	q := db.New(connection)
	accounts, err := q.CountAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := q.CountEncryptedKeys(ctx)
	if err != nil {
		return nil, err
	}
	webhooks, err := q.CountWebhookSecrets(ctx)
	if err != nil {
		return nil, err
	}
	// Key-only and webhook-only instances also depend on this file.
	return vault.Open(filepath.Join(directory, "credentials.key"), accounts == 0 && keys == 0 && webhooks == 0)
}
