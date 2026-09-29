package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	queries "github.com/murongg/SubLane/internal/storage/db"
)

func TestGrokMigrationPreservesAccountReferencesAndConstraints(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "011_grok.sql" {
			break
		}
		if err := apply(ctx, previous, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(1,'synthetic-owner','synthetic-hash','admin',1)`,
		`INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1)`,
		`INSERT INTO accounts(id,name,account_id,status,credential,created_at,updated_at) VALUES('synthetic-account','Synthetic account','synthetic-subject','ready',X'1234',1,1)`,
		`INSERT INTO account_groups(id,name,enabled,created_at,updated_at) VALUES(1,'Synthetic pool',1,1,1)`,
		`INSERT INTO group_accounts(group_id,account_id) VALUES(1,'synthetic-account')`,
		`INSERT INTO account_usage(account_id,snapshot,updated_at) VALUES('synthetic-account',X'1234',1)`,
	} {
		if _, err := previous.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previous.Close()
	connection, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for _, table := range []string{"accounts", "group_accounts", "account_usage"} {
		var count int
		if err := connection.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatal("lost reference", table, count, err)
		}
	}
	for _, statement := range []string{`UPDATE accounts SET provider='xai'`, `UPDATE accounts SET provider='codex'`} {
		if _, err := connection.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{`UPDATE accounts SET tenant_id=2`, `UPDATE accounts SET provider='unknown'`, `INSERT INTO group_accounts(group_id,account_id) VALUES(1,'missing')`} {
		if _, err := connection.Exec(statement); err == nil {
			t.Fatal("constraint lost", statement)
		}
	}
	var foreignKeys int
	if err := connection.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatal("FK disabled", err)
	}
	if _, err := connection.Exec(`UPDATE accounts SET provider='xai'`); err != nil {
		t.Fatal(err)
	}
	q := queries.New(connection)
	setup, err := q.GetSetupAccounts(ctx, 1)
	if err != nil || setup.Enabled != 1 || setup.Verified != 1 {
		t.Fatal("Grok not counted in setup", setup, err)
	}
	signals, err := q.ListAlertSignals(ctx, queries.ListAlertSignalsParams{TenantID: 1, Since: 1, Now: 10})
	if err != nil || len(signals) != 0 {
		t.Fatal("ready Grok pool reported unavailable", signals, err)
	}
	if _, err := connection.Exec(`UPDATE accounts SET status='reauth_required'`); err != nil {
		t.Fatal(err)
	}
	signals, err = q.ListAlertSignals(ctx, queries.ListAlertSignalsParams{TenantID: 1, Since: 1, Now: 10})
	if err != nil || len(signals) != 2 {
		t.Fatal("Grok reauthorization not reported", signals, err)
	}
}
