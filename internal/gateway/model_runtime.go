package gateway

import (
	"context"
	"database/sql"
	"errors"

	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

const maxModelRuntime = 1000

type modelKey struct{ account, model string }
type modelRuntime struct {
	key                                 modelKey
	lifecycle, runtimeRevision, version int64
	cooldownUntil, failures, inFlight   int64
	lastFailureSequence                 int64
	persistFailed                       bool
}

// Caller holds the admission lock. Idle persisted state is read on demand; only
// active leases and failed writes need memory, so normal traffic cannot grow a cache.
func (s *Service) readModelRuntime(ctx context.Context, q *db.Queries, id, model string) (*modelRuntime, error) {
	model = upstream.CatalogModelID(model)
	if model == "" {
		return nil, nil
	}
	row, err := q.GetAccountModelRuntime(ctx, db.GetAccountModelRuntimeParams{TargetID: id, WorkspaceID: s.tenantID, Model: model})
	if err != nil {
		return nil, err
	}
	key := modelKey{id, model}
	if state := s.modelHealth[key]; state != nil && state.lifecycle == row.Lifecycle && state.runtimeRevision == row.RuntimeRevision {
		return state, nil
	}
	return &modelRuntime{key: key, lifecycle: row.Lifecycle, runtimeRevision: row.RuntimeRevision, version: row.Version, cooldownUntil: row.CooldownUntil, failures: row.Failures}, nil
}

func (s *Service) modelAdmission(ctx context.Context, q *db.Queries, id, model string) error {
	state, err := s.readModelRuntime(ctx, q, id, model)
	if err != nil || state == nil {
		return err
	}
	if state.cooldownUntil > s.now().Unix() {
		return &CoolingError{RetryAfter: state.cooldownUntil - s.now().Unix(), Model: state.key.model}
	}
	if state.failures > 0 && state.inFlight > 0 {
		return ErrAccountBusy
	}
	return nil
}

func (s *Service) modelLease(ctx context.Context, id, model string) (*modelRuntime, error) {
	state, err := s.readModelRuntime(ctx, s.queries, id, model)
	if err != nil || state == nil {
		return state, err
	}
	if s.modelHealth[state.key] == nil && len(s.modelHealth) >= maxModelRuntime {
		return nil, ErrBusy
	}
	return state, nil
}

func (s *Service) finishModelRuntime(ctx context.Context, e *observation, outcome, retry string) bool {
	state := e.modelState
	if state == nil {
		return false
	}
	state.inFlight = max(0, state.inFlight-1)
	if s.modelHealth[state.key] != state {
		return false
	}
	failed := false
	if e.modelLimited {
		next := *state
		next.failures = min(16, next.failures+1)
		next.cooldownUntil = max(next.cooldownUntil, s.now().Unix()+retryDelay(retry, s.now()))
		s.sequence++
		next.lastFailureSequence = s.sequence
		version, err := s.queries.SaveAccountModelRuntime(ctx, db.SaveAccountModelRuntimeParams{TargetID: state.key.account, WorkspaceID: s.tenantID, Model: state.key.model, Lifecycle: state.lifecycle, RuntimeRevision: state.runtimeRevision, CooldownUntil: next.cooldownUntil, Failures: next.failures})
		if err == nil {
			next.version = version
			next.persistFailed = false
			*state = next
		} else if !errors.Is(err, sql.ErrNoRows) {
			// A failed write must not reopen a known limited model in this process.
			next.persistFailed = true
			*state = next
			failed = true
		}
	} else if (outcome == "success" || outcome == "incomplete") && state.cooldownUntil <= s.now().Unix() && state.lastFailureSequence < e.sequence && state.failures > 0 {
		// Only a newer successful request for this model may clear its failure.
		// Conditional deletion protects a later observation and authorization lifecycle.
		err := s.queries.DeleteAccountModelLimit(ctx, db.DeleteAccountModelLimitParams{TargetID: state.key.account, WorkspaceID: s.tenantID, Model: state.key.model, Version: state.version, Lifecycle: state.lifecycle, RuntimeRevision: state.runtimeRevision})
		if err == nil {
			state.failures = 0
			state.cooldownUntil = 0
			state.persistFailed = false
		} else {
			state.persistFailed = true
			failed = true
		}
	}
	if state.inFlight == 0 && !state.persistFailed {
		delete(s.modelHealth, state.key)
	}
	return failed
}
