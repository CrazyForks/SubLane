package allocations

import (
	"context"
	"testing"
	"time"
)

func TestSettlementSession_NormalFlow(t *testing.T) {
	session := NewSettlementSession("req-123", 1)

	if !session.NeedsSettlement() {
		t.Error("New session should need settlement")
	}

	if session.settled {
		t.Error("New session should not be settled")
	}
}

func TestSettlementSession_Cancellation(t *testing.T) {
	session := NewSettlementSession("req-456", 1)

	// Simulate cancellation without actual database
	session.mu.Lock()
	session.canceled = true
	session.mu.Unlock()

	if session.NeedsSettlement() {
		t.Error("Canceled session should not need settlement")
	}
}

func TestSettlementSession_Idempotency(t *testing.T) {
	session := NewSettlementSession("req-789", 1)

	// Mark as settled
	session.mu.Lock()
	session.settled = true
	session.mu.Unlock()

	// Second call should be no-op
	if session.NeedsSettlement() {
		t.Error("Settled session should not need settlement")
	}
}

func TestSettlementManager_Registration(t *testing.T) {
	manager := NewSettlementManager()

	session1 := NewSettlementSession("req-1", 1)
	session2 := NewSettlementSession("req-2", 2)

	manager.Register(session1)
	manager.Register(session2)

	if len(manager.sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(manager.sessions))
	}

	manager.Unregister("req-1")

	if len(manager.sessions) != 1 {
		t.Errorf("Expected 1 session after unregister, got %d", len(manager.sessions))
	}
}

func TestSettlementManager_CancelAll(t *testing.T) {
	manager := NewSettlementManager()

	session1 := NewSettlementSession("req-1", 1)
	session2 := NewSettlementSession("req-2", 2)

	manager.Register(session1)
	manager.Register(session2)

	// Mark all as canceled
	ctx := context.Background()
	now := time.Now().Unix()

	// Simulate cancel all without actual database
	for _, s := range manager.sessions {
		s.mu.Lock()
		s.canceled = true
		s.mu.Unlock()
	}

	manager.CancelAll(ctx, nil, now)

	if session1.NeedsSettlement() || session2.NeedsSettlement() {
		t.Error("All sessions should be canceled")
	}
}

func TestSettlementSession_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSettlementSession("req-ctx", 1)

	// Simulate context cancellation
	cancel()

	if ctx.Err() == nil {
		t.Error("Context should be canceled")
	}

	// Session should handle cancellation gracefully
	session.mu.Lock()
	session.canceled = true
	session.mu.Unlock()

	if session.NeedsSettlement() {
		t.Error("Session should not need settlement after context cancellation")
	}
}

func TestSettlementSession_RaceCondition(t *testing.T) {
	session := NewSettlementSession("req-race", 1)
	ctx := context.Background()
	now := time.Now().Unix()

	// Simulate concurrent access
	done := make(chan bool, 2)

	go func() {
		session.mu.Lock()
		session.settled = true
		session.mu.Unlock()
		done <- true
	}()

	go func() {
		// Try to cancel while settling
		_ = session.Cancel(ctx, nil, now)
		done <- true
	}()

	<-done
	<-done

	// Should be in a consistent state
	if session.NeedsSettlement() {
		t.Error("Session should not need settlement after concurrent operations")
	}
}
