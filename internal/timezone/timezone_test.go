package timezone

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
)

func TestPersistedInstanceTimeZone(t *testing.T) {
	ctx := context.Background()
	conn, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-pass", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	svc, err := New(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Name() != "UTC" || svc.Location().String() != "UTC" {
		t.Fatal("default must be UTC")
	}
	for _, invalid := range []string{"", "Local", "Not/AZone", "../UTC"} {
		if _, err := svc.Save(ctx, invalid); !errors.Is(err, ErrInput) {
			t.Fatalf("%q: %v", invalid, err)
		}
	}
	actor := audit.WithActor(ctx, audit.Actor{TenantID: 1, ID: 1, Username: "synthetic-admin", Role: "admin", Source: "user"})
	if name, err := svc.Save(actor, "Asia/Shanghai"); err != nil || name != "Asia/Shanghai" {
		t.Fatal(name, err)
	}
	if svc.Location().String() != "Asia/Shanghai" {
		t.Fatal("location was not published")
	}
	reloaded, err := New(ctx, conn)
	if err != nil || reloaded.Name() != "Asia/Shanghai" {
		t.Fatal(reloaded, err)
	}
	var events int
	if err := conn.QueryRow(`SELECT count(*) FROM audit_events WHERE action='settings.update' AND resource_id='timezone'`).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
}
