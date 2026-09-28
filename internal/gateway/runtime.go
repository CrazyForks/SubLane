package gateway

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

var ErrAccountBusy = errors.New("account_busy")
var ErrAccountCooling = errors.New("account_cooling")

type CoolingError struct{ RetryAfter int64 }

func (e *CoolingError) Error() string { return ErrAccountCooling.Error() }
func (e *CoolingError) Unwrap() error { return ErrAccountCooling }

type Runtime struct {
	lastFailureSequence int64
	ID                  string `json:"id"`
	MaxConcurrency      int64  `json:"max_concurrency"`
	InFlight            int64  `json:"in_flight"`
	CooldownUntil       int64  `json:"cooldown_until"`
	Reason              string `json:"reason"`
	Failures            int64  `json:"failures"`
	State               string `json:"state"`
	QuotaState          string `json:"quota_state"`
	lastFailureAt       int64
	revision            int64
}

// Caller holds s.mu. Cached state keeps a failed persistence attempt from reopening a cooling account.
func (s *Service) loadRuntime(ctx context.Context) error {
	rows, err := s.queries.ListAccountRuntime(ctx, s.tenantID)
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(rows))
	for _, row := range rows {
		present[row.ID] = true
		state := s.health[row.ID]
		if state == nil {
			state = &Runtime{ID: row.ID, CooldownUntil: row.CooldownUntil, Reason: row.Reason, Failures: row.Failures, lastFailureAt: row.LastFailureAt, revision: row.Revision}
			s.health[row.ID] = state
		}
		state.MaxConcurrency = row.MaxConcurrency
	}
	for id, state := range s.health {
		if !present[id] && state.InFlight == 0 {
			delete(s.health, id)
		}
	}
	return nil
}
func (s *Service) Runtime(ctx context.Context) ([]Runtime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadRuntime(ctx); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListAccountRuntime(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]Runtime, 0, len(rows))
	now := s.now().Unix()
	for _, row := range rows {
		state := *s.health[row.ID]
		state.State = "available"
		if state.CooldownUntil > now {
			state.State = "cooling"
		} else if state.CooldownUntil > 0 && state.Failures > 0 {
			state.State = "retry_ready"
			if state.InFlight > 0 {
				state.State = "probing"
			}
		}
		account, err := s.accounts.Get(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		quota, err := s.accountQuota(ctx, s.queries, account)
		if err != nil {
			return nil, err
		}
		state.QuotaState = quota.State
		if state.State == "available" && quota.State == "exhausted" {
			state.State = "quota_exhausted"
		}
		result = append(result, state)
	}
	return result, nil
}
func (s *Service) SetConcurrency(ctx context.Context, id string, limit int64) error {
	if limit < 1 || limit > 30 {
		return accounts.ErrInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	n, err := q.SetAccountConcurrency(ctx, db.SetAccountConcurrencyParams{ID: id, TenantID: s.tenantID, MaxConcurrency: limit, Now: s.now().Unix()})
	if err != nil {
		return err
	}
	if n == 0 {
		return accounts.ErrNotFound
	}
	if err := audit.Record(ctx, q, "account.concurrency", "account", id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if state := s.health[id]; state != nil {
		state.MaxConcurrency = limit
	}
	return nil
}
func (s *Service) Resume(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadRuntime(ctx); err != nil {
		return err
	}
	state := s.health[id]
	if state == nil {
		return accounts.ErrNotFound
	}
	next := *state
	next.CooldownUntil = 0
	next.Reason = ""
	next.Failures = 0
	next.lastFailureAt = 0
	next.lastFailureSequence = 0
	next.revision++
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if err := q.SaveAccountRuntime(ctx, s.runtimeParams(&next)); err != nil {
		return err
	}
	if err := audit.Record(ctx, q, "account.resume", "account", id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*state = next
	return nil
}
func (s *Service) persistRuntime(ctx context.Context, state *Runtime) error {
	return s.queries.SaveAccountRuntime(ctx, s.runtimeParams(state))
}
func (s *Service) runtimeParams(state *Runtime) db.SaveAccountRuntimeParams {
	return db.SaveAccountRuntimeParams{AccountID: state.ID, TenantID: s.tenantID, CooldownUntil: state.CooldownUntil, Reason: state.Reason, Failures: state.Failures, LastFailureAt: state.lastFailureAt, Revision: state.revision}
}
func (s *Service) accountAdmission(account accounts.Account) error {
	if !account.Enabled {
		return accounts.ErrDisabled
	}
	if account.Status == "reauth_required" {
		return accounts.ErrReauthorize
	}
	state := s.health[account.ID]
	if state == nil {
		return accounts.ErrNotFound
	}
	if state.CooldownUntil > s.now().Unix() {
		return &CoolingError{RetryAfter: state.CooldownUntil - s.now().Unix()}
	}
	if state.InFlight >= state.MaxConcurrency || (state.CooldownUntil > 0 && state.Failures > 0 && state.InFlight > 0) {
		return ErrAccountBusy
	}
	return nil
}
func retryDelay(value string, now time.Time) int64 {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return max(1, min(3600, seconds))
	}
	if until, err := http.ParseTime(value); err == nil {
		return max(1, min(3600, int64(math.Ceil(until.Sub(now).Seconds()))))
	}
	return 60
}
