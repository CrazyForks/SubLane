package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
)

var ErrQuotaExhausted = errors.New("quota_exhausted")

type QuotaError struct{ RetryAfter int64 }

func (e *QuotaError) Error() string { return ErrQuotaExhausted.Error() }
func (e *QuotaError) Unwrap() error { return ErrQuotaExhausted }

type quotaDecision struct {
	State   string
	RetryAt int64
}

func quotaStatus(value upstream.Usage, now time.Time) quotaDecision {
	result := quotaDecision{State: "unknown"}
	if value.UpdatedAt <= 0 {
		return result
	}
	state := snapshotState(&usageEntry{snapshot: &value}, now)
	if state.Stale {
		result.State = "stale"
		return result
	}
	// Additional named limits are not a documented mapping to model IDs. Only the main
	// subscription limit can exclude the whole account; partial/absent data stays unknown.
	for _, limit := range value.Limits {
		if limit.Name != "" {
			continue
		}
		blocked := (limit.Allowed != nil && !*limit.Allowed) || (limit.LimitReached != nil && *limit.LimitReached)
		known := (limit.Allowed != nil && *limit.Allowed) || (limit.LimitReached != nil && !*limit.LimitReached)
		for _, window := range limit.Windows {
			if window.ResetAt != nil && *window.ResetAt <= now.Unix() {
				return quotaDecision{State: "stale"}
			}
			if window.UsedPercent != nil {
				known = true
				blocked = blocked || *window.UsedPercent >= 100
			}
		}
		if blocked {
			return quotaDecision{State: "exhausted", RetryAt: state.ExpiresAt}
		}
		if known {
			result.State = "available"
		}
	}
	return result
}

func savedUsage(row db.GetAccountUsageRow) *upstream.Usage {
	var value upstream.Usage
	if row.UpdatedAt == nil || len(row.Snapshot) > 128<<10 || json.Unmarshal(row.Snapshot, &value) != nil || value.Limits == nil || value.UpdatedAt != *row.UpdatedAt || value.UpdatedAt <= 0 {
		return nil
	}
	return &value
}

func (s *Service) accountQuota(ctx context.Context, q *db.Queries, account accounts.Account, model string) (quotaDecision, error) {
	if !quotaRoutingSupported(account.Provider) {
		return quotaDecision{State: "unsupported"}, nil
	}
	row, err := q.GetAccountUsage(ctx, account.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return quotaDecision{State: "unknown"}, nil
	}
	if err != nil {
		return quotaDecision{}, err
	}
	if value := savedUsage(row); value != nil {
		if account.Provider == "antigravity" {
			// Only an exact native model match can deny a request. Never infer an
			// account-wide limit or a mapping from another model's name.
			id := upstream.CatalogModelID(model)
			for _, limit := range value.Limits {
				if id != "" && limit.Name == id {
					limit.Name = ""
					selected := *value
					selected.Limits = []upstream.UsageLimit{limit}
					return quotaStatus(selected, s.now()), nil
				}
			}
			return quotaDecision{State: "unknown"}, nil
		}
		return quotaStatus(*value, s.now()), nil
	}
	return quotaDecision{State: "unknown"}, nil
}
func (s *Service) quotaAdmission(ctx context.Context, q *db.Queries, account accounts.Account, model string) error {
	decision, err := s.accountQuota(ctx, q, account, model)
	if err != nil {
		return err
	}
	if decision.State == "exhausted" {
		return &QuotaError{RetryAfter: max(1, decision.RetryAt-s.now().Unix())}
	}
	return nil
}

// Traffic warms the existing shared cache without awaiting provider IO. Reading quota
// here (before selection opens its SQLite transaction) avoids a cache/DB lock inversion.
func (s *Service) warmUsage(ctx context.Context, userID, groupID int64, provider string) error {
	if provider != "" && !quotaRoutingSupported(provider) {
		return nil
	}
	if _, err := poolAccounts(ctx, s.queries, userID, groupID); err != nil {
		return err
	}
	rows, err := s.queries.ListGroupCatalogs(ctx, groupID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if quotaRoutingSupported(row.Provider) && (provider == "" || row.Provider == provider) {
			if _, err := s.usageSnapshot(ctx, row.ID, false, false); err != nil && ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	return nil
}

func quotaRoutingSupported(provider string) bool {
	return provider == "codex" || provider == "claude" || provider == "antigravity"
}
