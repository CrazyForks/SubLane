package allocations

import (
	"context"
	"sync"

	"github.com/murongg/SubLane/internal/storage/db"
)

// SettlementSession tracks the lifecycle of an allocation entry and ensures proper settlement.
// Inspired by new-api's BillingSession pattern.
type SettlementSession struct {
	requestID string
	schemeID  int64
	mu        sync.Mutex
	settled   bool
	canceled  bool
}

// NewSettlementSession creates a new settlement session for tracking allocation lifecycle.
func NewSettlementSession(requestID string, schemeID int64) *SettlementSession {
	return &SettlementSession{
		requestID: requestID,
		schemeID:  schemeID,
	}
}

// Settle marks the allocation entry as settled with the given completion data.
// This operation is idempotent - multiple calls are safe.
func (s *SettlementSession) Settle(ctx context.Context, q *db.Queries, completion Completion, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.canceled {
		return nil // Already settled or canceled, no-op
	}

	if err := Finish(ctx, q, s.requestID, completion, now, false); err != nil {
		return err
	}

	s.settled = true
	return nil
}

// Cancel settles the allocation entry with zero usage.
// This is called when a request is canceled or fails before completion.
// This operation is idempotent - multiple calls are safe.
func (s *SettlementSession) Cancel(ctx context.Context, q *db.Queries, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.canceled {
		return nil // Already settled or canceled, no-op
	}

	// Settle with zero usage
	completion := Completion{
		Input:      0,
		Output:     0,
		Cached:     0,
		Known:      true,
		Dispatched: false,
		Rejected:   true,
	}

	if err := Finish(ctx, q, s.requestID, completion, now, false); err != nil {
		return err
	}

	s.canceled = true
	return nil
}

// NeedsSettlement returns true if the session has not been settled yet.
func (s *SettlementSession) NeedsSettlement() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.settled && !s.canceled
}

// SettlementManager manages multiple settlement sessions.
type SettlementManager struct {
	mu       sync.Mutex
	sessions map[string]*SettlementSession
}

// NewSettlementManager creates a new settlement manager.
func NewSettlementManager() *SettlementManager {
	return &SettlementManager{
		sessions: make(map[string]*SettlementSession),
	}
}

// Register adds a settlement session to the manager.
func (m *SettlementManager) Register(session *SettlementSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.requestID] = session
}

// Unregister removes a settlement session from the manager.
func (m *SettlementManager) Unregister(requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, requestID)
}

// CancelAll cancels all registered settlement sessions.
func (m *SettlementManager) CancelAll(ctx context.Context, q *db.Queries, now int64) {
	m.mu.Lock()
	sessions := make([]*SettlementSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()

	for _, session := range sessions {
		_ = session.Cancel(ctx, q, now)
	}
}
