package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

var ErrTestCooling = errors.New("alert_test_cooling")
var ErrCapacityUnavailable = errors.New("alert_capacity_unavailable")

func (s *Service) Test(ctx context.Context, tenantID int64) error {
	s.mu.Lock()
	raw, err := s.queries.GetAlertConfig(ctx, tenantID)
	var cfg configuration
	if err == nil {
		err = json.Unmarshal([]byte(raw), &cfg)
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if len(cfg.Endpoint) == 0 {
		s.mu.Unlock()
		return ErrInput
	}
	if cfg.TestedAt > 0 && cfg.TestedAt+60 > s.now().Unix() {
		s.mu.Unlock()
		return ErrTestCooling
	}
	plain, err := s.vault.OpenWebhook(tenantID, cfg.Endpoint)
	if err != nil {
		s.mu.Unlock()
		return ErrDelivery
	}
	cfg.TestedAt = s.now().Unix()
	encoded, err := json.Marshal(cfg)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err == nil {
		q := s.queries.WithTx(tx)
		_, err = q.SaveAlertConfig(ctx, db.SaveAlertConfigParams{TenantID: tenantID, Config: string(encoded)})
		if err == nil {
			err = audit.Record(ctx, q, "settings.test", "settings", "alerts")
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	s.testFlights[tenantID] = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); delete(s.testFlights, tenantID); s.mu.Unlock() }()
	// Reserve the persisted cooldown before IO, including failed delivery and restart.
	return deliver(call, s.sender, string(plain), Event{ID: newEventID(), WorkspaceID: tenantID, Kind: "test", Subject: "workspace", Status: "test", ObservedAt: cfg.TestedAt})
}
