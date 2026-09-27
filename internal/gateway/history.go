package gateway

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

type requestKey struct{}
type requestIdentity struct {
	RequestID string
	KeyID     int64
	Transport string
}

func WithRequestIdentity(ctx context.Context, keyID int64, transport string) context.Context {
	if transport != "websocket" {
		transport = "http"
	}
	identity, _ := ctx.Value(requestKey{}).(requestIdentity)
	// Native routes reserve an ID before authentication. Only that pending identity may be reused.
	if identity.RequestID == "" || identity.KeyID != 0 || identity.Transport != transport {
		identity.RequestID = "req_" + rand.Text()
	}
	identity.KeyID, identity.Transport = keyID, transport
	return context.WithValue(ctx, requestKey{}, identity)
}

var safeRequestID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func RequestID(ctx context.Context) string {
	identity, _ := ctx.Value(requestKey{}).(requestIdentity)
	return identity.RequestID
}

var safeModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/()+-]{0,159}$`)

type observation struct {
	kind               Kind
	service            *Service
	ctx                context.Context
	cancel             context.CancelFunc
	stopParent         func() bool
	once               sync.Once
	started            time.Time
	record             db.RecordRequestParams
	revision           int64
	leased             bool
	memberLeased       bool
	schemeID           int64
	allocationTracked  bool
	upstreamDispatched bool
	sequence           int64
	quotaReadStartedAt int64
	quotaRevision      int64
	quota              *upstream.Usage
}

func (s *Service) begin(ctx context.Context, userID, groupID int64, kind Kind) (*observation, error) {
	if _, err := s.queries.GetTenantGroup(ctx, db.GetTenantGroupParams{ID: groupID, TenantID: s.tenantID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, groups.ErrUnavailable
		}
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, context.Canceled
	}
	s.workers.Add(1)
	s.sequence++
	operation, cancel := context.WithCancel(ctx)
	identity, _ := ctx.Value(requestKey{}).(requestIdentity)
	if identity.RequestID == "" {
		identity.RequestID = "req_" + rand.Text()
	}
	if identity.Transport == "" {
		identity.Transport = "http"
	}
	name := "responses"
	if kind == Chat {
		name = "chat"
	} else if kind == Compact {
		name = "compact"
	} else if kind == Messages {
		name = "messages"
	} else if kind.IsGemini() {
		name = "gemini"
	}
	started := s.now()
	return &observation{kind: kind, sequence: s.sequence, service: s, ctx: operation, cancel: cancel, stopParent: context.AfterFunc(s.runContext, cancel), started: started, record: db.RecordRequestParams{RequestID: identity.RequestID, UserID: userID, GroupID: groupID, KeyID: identity.KeyID, Transport: identity.Transport, Operation: name, StartedAt: started.Unix()}}, nil
}
func (e *observation) finish(outcome, code, penalty, retry string) {
	e.once.Do(func() {
		defer e.service.workers.Done()
		defer e.cancel()
		defer e.stopParent()
		e.record.Outcome = outcome
		e.record.ErrorCode = code
		e.record.DurationMs = max(0, e.service.now().Sub(e.started).Milliseconds())
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s := e.service
		if e.quota != nil && (outcome == "success" || outcome == "incomplete") {
			if err := s.observeUsage(ctx, e.record.AccountID, e.quotaRevision, *e.quota); err != nil {
				slog.Warn("Unable to persist response quota")
			}
		}
		s.mu.Lock()
		if e.memberLeased {
			if s.memberActive[e.record.UserID] <= 1 {
				delete(s.memberActive, e.record.UserID)
			} else {
				s.memberActive[e.record.UserID]--
			}
		}
		if e.leased {
			if state := s.health[e.record.AccountID]; state != nil {
				changed := false
				if state.revision == e.revision {
					if penalty != "" {
						state.Failures = min(16, state.Failures+1)
						state.lastFailureAt = s.now().UnixMilli()
						s.sequence++
						state.lastFailureSequence = s.sequence
						state.Reason = penalty
						changed = true
						delay := int64(0)
						if penalty == "rate_limited" {
							delay = retryDelay(retry, s.now())
						} else if state.Failures >= 3 {
							delay = min(300, int64(30)<<min(4, state.Failures-3))
						}
						if delay > 0 {
							state.CooldownUntil = max(state.CooldownUntil, s.now().Unix()+delay)
						}
						// Sequence order, rather than wall-clock milliseconds, protects against late successes in the same tick.
					} else if (outcome == "success" || outcome == "incomplete") && state.lastFailureSequence < e.sequence && state.CooldownUntil <= s.now().Unix() {
						changed = state.Failures != 0 || state.CooldownUntil != 0
						state.Failures = 0
						state.CooldownUntil = 0
						state.Reason = ""
						state.lastFailureAt = 0
						state.lastFailureSequence = 0
					}
					if changed {
						if err := s.persistRuntime(ctx, state); err != nil {
							slog.Error("Unable to persist account cooldown", "account_id", state.ID)
						}
					}
				}
				state.InFlight = max(0, state.InFlight-1)
			}
		}
		// Admission must not observe released leases before the token charge commits.
		defer s.mu.Unlock()
		tx, err := s.db.BeginTx(ctx, nil)
		if err == nil {
			defer tx.Rollback()
			q := s.queries.WithTx(tx)
			err = e.settleAllocation(ctx, q)
			if err == nil {
				err = q.RecordRequest(ctx, e.record)
			}
			if err == nil {
				err = recordStatistics(ctx, q, e.record)
			}
			if err == nil {
				err = allocations.Prune(ctx, q, s.now().Add(-90*24*time.Hour).Unix())
			}
			if err == nil {
				err = q.PruneStatistics(ctx, s.now().UTC().Truncate(24*time.Hour).Unix()-89*86400)
			}
			if err == nil {
				err = q.PruneHourlyUsage(ctx, s.now().UTC().Truncate(24*time.Hour).Unix()-89*86400)
			}
			if err == nil {
				err = q.PruneRequests(ctx, s.now().Add(-7*24*time.Hour).Unix())
			}
			if err == nil {
				err = tx.Commit()
			}
		}
		if err != nil {
			if e.allocationTracked {
				s.allocationFailure = true
			}
			slog.Error("Unable to persist request metadata")
		}
	})
}
func classify(ctx context.Context, err error) (outcome, code, penalty string) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "canceled", "client_disconnected", ""
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "error", "timeout", "timeout"
	}
	var rejected *upstream.UpstreamError
	if errors.As(err, &rejected) {
		return statusOutcome(rejected.Status)
	}
	switch {
	case errors.Is(err, allocations.ErrQuota), errors.Is(err, allocations.ErrPending), errors.Is(err, allocations.ErrRisk), errors.Is(err, allocations.ErrUnavailable), errors.Is(err, allocations.ErrUnpriced):
		return "rejected", err.Error(), ""
	case errors.Is(err, ErrMemberBusy):
		return "rejected", "member_busy", ""
	case errors.Is(err, ErrMemberRate):
		return "rejected", "member_rate_limited", ""
	case errors.Is(err, ErrAllocationAccounting):
		return "error", "allocation_accounting_unavailable", ""
	case errors.Is(err, ErrQuotaExhausted):
		return "rejected", "quota_exhausted", ""
	case errors.Is(err, ErrAccountBusy):
		return "rejected", "account_busy", ""
	case errors.Is(err, ErrAccountCooling):
		return "rejected", "account_cooling", ""
	case errors.Is(err, ErrNoAccount), errors.Is(err, ErrAffinityUnavailable), errors.Is(err, accounts.ErrDisabled), errors.Is(err, accounts.ErrNotFound):
		return "rejected", "account_unavailable", ""
	case errors.Is(err, groups.ErrUnavailable):
		return "rejected", "group_unavailable", ""
	case errors.Is(err, accounts.ErrReauthorize):
		return "error", "auth_required", ""
	case errors.Is(err, ErrModelUnavailable):
		return "rejected", "model_not_available", ""
	case errors.Is(err, ErrCatalogUnavailable):
		return "rejected", "model_catalog_unavailable", ""
	case errors.Is(err, ErrModelNotAllowed):
		return "rejected", "model_not_allowed", ""
	case errors.Is(err, upstream.ErrInput), errors.Is(err, upstream.ErrContinuation):
		return "rejected", "invalid_request", ""
	case errors.Is(err, upstream.ErrInterrupted), errors.Is(err, upstream.ErrResponse):
		return "error", "stream_interrupted", "upstream_error"
	case errors.Is(err, accounts.ErrRefresh):
		return "error", "refresh_failed", "upstream_error"
	case errors.Is(err, upstream.ErrUpstream):
		return "error", "upstream_unavailable", "upstream_error"
	default:
		return "error", "request_failed", ""
	}
}
func (e *observation) fail(err error) {
	var rejected *upstream.UpstreamError
	if errors.As(err, &rejected) {
		status := int64(rejected.Status)
		e.record.UpstreamStatus = &status
		outcome, code, penalty := statusOutcome(rejected.Status)
		e.finish(outcome, code, penalty, rejected.RetryAfter)
		return
	}
	outcome, code, penalty := classify(e.ctx, err)
	e.finish(outcome, code, penalty, "")
}

func (e *observation) observe(raw []byte) {
	type usage struct {
		Input   *int64 `json:"input_tokens"`
		Output  *int64 `json:"output_tokens"`
		Details struct {
			Cached *int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	}
	var value struct {
		Response struct {
			Usage *usage `json:"usage"`
		} `json:"response"`
		Usage *usage `json:"usage"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	tokens := value.Response.Usage
	if tokens == nil {
		tokens = value.Usage
	}
	if tokens == nil {
		return
	}
	valid := func(n *int64) *int64 {
		if n == nil || *n < 0 || *n > 1_000_000_000 {
			return nil
		}
		return n
	}
	e.record.InputTokens = valid(tokens.Input)
	e.record.OutputTokens = valid(tokens.Output)
	e.record.CachedTokens = valid(tokens.Details.Cached)
}
