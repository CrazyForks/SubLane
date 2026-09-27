package gateway

import (
	"context"
	"database/sql"
	"errors"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
)

var ErrAllocationAccounting = errors.New("allocation_accounting_unavailable")

func (s *Service) Allocations() *allocations.Service {
	service := allocations.NewForTenantWithPricing(s.db, s.tenantID, s.pricing)
	service.SetLocation(s.location)
	return service
}

func (s *Service) Pricing() *pricing.Service { return s.pricing }
func (e *observation) settleAllocation(ctx context.Context, q *db.Queries) error {
	if !e.allocationTracked {
		return nil
	}
	value := func(v *int64) int64 {
		if v != nil {
			return *v
		}
		return 0
	}
	known := e.record.InputTokens != nil && e.record.OutputTokens != nil && (e.record.Outcome == "success" || e.record.Outcome == "incomplete")
	rejected := e.record.UpstreamStatus != nil && *e.record.UpstreamStatus >= 400
	err := allocations.Finish(ctx, q, e.record.RequestID, allocations.Completion{Input: value(e.record.InputTokens), Output: value(e.record.OutputTokens), Cached: value(e.record.CachedTokens), Known: known, Dispatched: e.upstreamDispatched, Rejected: rejected}, e.service.now().UnixMilli(), false)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAllocationAccounting
	}
	return err
}

func (s *Service) PrepareAllocations(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoverAllocations(ctx)
}

func (s *Service) prepareAllocation(ctx context.Context, e *observation) error {
	if err := s.recoverAllocations(ctx); err != nil {
		return err
	}
	scheme, err := allocations.KeyScheme(ctx, s.queries, e.record.KeyID, e.record.UserID, e.record.GroupID, s.now().Unix())
	if err != nil {
		return err
	}
	e.schemeID = scheme
	return nil
}

// Recover interrupted allocation requests before admitting new work after a restart.
func (s *Service) recoverAllocations(ctx context.Context) error {
	if s.allocationFailure {
		return ErrAllocationAccounting
	}
	if s.allocationsReady {
		return nil
	}
	if err := s.queries.RecoverAllocationEntries(ctx, s.tenantID); err != nil {
		return ErrAllocationAccounting
	}
	s.allocationsReady = true
	return nil
}
