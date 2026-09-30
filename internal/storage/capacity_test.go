package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCapacityAlertMigrationPreservesDeliveryHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	previous, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "013_capacity_alerts.sql" {
			break
		}
		if err := apply(ctx, previous, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(1,'synthetic-owner','synthetic-hash','admin',1)`,
		`INSERT INTO tenants(id,name,owner_user_id,created_at) VALUES(1,'Synthetic workspace',1,1)`,
		`INSERT INTO alert_states VALUES(1,'request_failures','workspace',1,0,'11111111111111111111111111111111',1900000000,2,1900000060,0,1)`,
	} {
		if _, err := previous.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := previous.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var active, attempts, retry, failed int64
	if err := upgraded.QueryRow(`SELECT active,attempts,retry_at,delivery_failed FROM alert_states WHERE tenant_id=1 AND kind='request_failures'`).Scan(&active, &attempts, &retry, &failed); err != nil || active != 1 || attempts != 2 || retry != 1900000060 || failed != 1 {
		t.Fatal(active, attempts, retry, failed, err)
	}
	if _, err := upgraded.Exec(`INSERT INTO alert_states VALUES(1,'model_unavailable','7:synthetic-model',1,0,'22222222222222222222222222222222',1900000000,0,0,0,0)`); err != nil {
		t.Fatal(err)
	}
}
