package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenInitializesSchemaAndPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "synthetic # database.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal=%s err=%v", journal, err)
	}
	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d err=%v", foreignKeys, err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "synthetic-setting", "synthetic-value"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var value string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = ?", "synthetic-setting").Scan(&value); err != nil || value != "synthetic-value" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	var migrations int
	wantMigrations, versionErr := CurrentSchemaVersion()
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || versionErr != nil || migrations != wantMigrations {
		t.Fatalf("migrations=%d want=%d err=%v versionErr=%v", migrations, wantMigrations, err, versionErr)
	}
}

func TestOpenAddsInvitationMigrationToExistingSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-previous.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := migrations.ReadFile("migrations/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ExecContext(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY NOT NULL, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ExecContext(ctx, string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES('001_schema.sql'); INSERT INTO settings(key,value) VALUES('synthetic.migration','retained')`); err != nil {
		t.Fatal(err)
	}
	if err := previous.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var retained string
	if err := upgraded.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='synthetic.migration'`).Scan(&retained); err != nil || retained != "retained" {
		t.Fatalf("previous data: %q %v", retained, err)
	}
	var table string
	if err := upgraded.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='invitations'`).Scan(&table); err != nil || table != "invitations" {
		t.Fatalf("invitation table: %q %v", table, err)
	}
	var applied int
	if err := upgraded.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name='002_invitations.sql'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("migration record: %d %v", applied, err)
	}
}

const formerProxySchema = `CREATE TABLE proxies (
 id TEXT PRIMARY KEY NOT NULL,
 tenant_id INTEGER NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL,
 address BLOB NOT NULL CHECK(length(address) <= 8192),
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 UNIQUE(tenant_id, name)
);
ALTER TABLE accounts ADD COLUMN proxy_id TEXT REFERENCES proxies(id) ON DELETE RESTRICT;
CREATE INDEX accounts_proxy_id_idx ON accounts(proxy_id) WHERE proxy_id IS NOT NULL;`

const formerProxyChecks = `ALTER TABLE proxies ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN checked_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN reachable INTEGER NOT NULL DEFAULT 0 CHECK(reachable IN (0,1));
ALTER TABLE proxies ADD COLUMN exit_ip TEXT NOT NULL DEFAULT '' CHECK(length(exit_ip) <= 45);
ALTER TABLE proxies ADD COLUMN country TEXT NOT NULL DEFAULT '' CHECK(length(country) <= 2);
ALTER TABLE proxies ADD COLUMN region TEXT NOT NULL DEFAULT '' CHECK(length(region) <= 128);
ALTER TABLE proxies ADD COLUMN city TEXT NOT NULL DEFAULT '' CHECK(length(city) <= 128);
ALTER TABLE proxies ADD COLUMN latency_ms INTEGER NOT NULL DEFAULT 0 CHECK(latency_ms BETWEEN 0 AND 30000);
ALTER TABLE proxies ADD COLUMN check_error TEXT NOT NULL DEFAULT '' CHECK(length(check_error) <= 32);`

func formerProxyDatabase(t *testing.T, path string, withChecks bool) {
	t.Helper()
	ctx := context.Background()
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ExecContext(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY NOT NULL, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_schema.sql", "002_invitations.sql"} {
		data, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := previous.ExecContext(ctx, string(data)); err != nil {
			t.Fatal(name, err)
		}
		if _, err := previous.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES(?)", name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := previous.ExecContext(ctx, formerProxySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES('003_proxies.sql')"); err != nil {
		t.Fatal(err)
	}
	if withChecks {
		if _, err := previous.ExecContext(ctx, formerProxyChecks); err != nil {
			t.Fatal(err)
		}
		if _, err := previous.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES('004_proxy_checks.sql')"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := previous.ExecContext(ctx, `INSERT INTO users(id,username,role,password_hash,enabled,created_at) VALUES(1,'synthetic-admin','admin','synthetic-hash',1,1); INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1); INSERT INTO proxies(id,tenant_id,name,address,created_at,updated_at) VALUES('synthetic-proxy',1,'Synthetic exit',x'01',1,1)`); err != nil {
		t.Fatal(err)
	}
	if withChecks {
		if _, err := previous.ExecContext(ctx, `UPDATE proxies SET checked_at=123,reachable=1,exit_ip='203.0.113.8',country='US' WHERE id='synthetic-proxy'`); err != nil {
			t.Fatal(err)
		}
	}
	if err := previous.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAddsProxyCheckColumnsToExistingProxySchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-previous-proxy.db")
	formerProxyDatabase(t, path, false)
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var name string
	var revision, checkedAt, reachable int64
	if err := upgraded.QueryRowContext(ctx, `SELECT name,revision,checked_at,reachable FROM proxies WHERE id='synthetic-proxy'`).Scan(&name, &revision, &checkedAt, &reachable); err != nil || name != "Synthetic exit" || revision != 0 || checkedAt != 0 || reachable != 0 {
		t.Fatalf("proxy migration: %q %d %d %d %v", name, revision, checkedAt, reachable, err)
	}
	var count int
	if err := upgraded.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 4 {
		t.Fatalf("merged migration count: %d %v", count, err)
	}
}

func TestOpenNormalizesFormerProxyCheckMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-four-step.db")
	formerProxyDatabase(t, path, true)
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var checkedAt int64
	var exitIP string
	if err := upgraded.QueryRowContext(ctx, `SELECT checked_at,exit_ip FROM proxies WHERE id='synthetic-proxy'`).Scan(&checkedAt, &exitIP); err != nil || checkedAt != 123 || exitIP != "203.0.113.8" {
		t.Fatalf("former proxy data: %d %q %v", checkedAt, exitIP, err)
	}
	var count int
	if err := upgraded.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 4 {
		t.Fatalf("former migration record retained: %d %v", count, err)
	}
}

func TestValidateSnapshotAcceptsFormerProxyCheckHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-four-step-backup.db")
	formerProxyDatabase(t, path, true)
	connection, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	version, err := ValidateSnapshot(ctx, connection)
	if err != nil || version != 4 {
		t.Fatalf("former backup history: %d %v", version, err)
	}
	current, err := CurrentSchemaVersion()
	if err != nil || current != 4 {
		t.Fatalf("merged current version: %d %v", current, err)
	}
}

func TestOpenExplainsUnsupportedPrereleaseDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY, applied_at TEXT);
		INSERT INTO schema_migrations(name,applied_at) VALUES('001_settings.sql','synthetic-time');
		CREATE TABLE account_affinity(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "older pre-release schema") {
		t.Fatalf("legacy database did not explain startup failure: %v", err)
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var count int
	if err := check.QueryRow(`SELECT count(*) FROM schema_migrations WHERE name='001_schema.sql'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy database was modified during rejection: count=%d err=%v", count, err)
	}
}
