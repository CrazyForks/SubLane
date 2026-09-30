package alerts

import (
	"context"
	"strconv"

	"github.com/murongg/SubLane/internal/capacity"
	"github.com/murongg/SubLane/internal/storage/db"
)

func (s *Service) SetCapacityObserver(observer func(context.Context, int64) ([]capacity.Pool, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capacityObserver = observer
}

func observeCapacity(ctx context.Context, observer func(context.Context, int64) ([]capacity.Pool, error), tenantID int64, cfg configuration) ([]db.ListAlertSignalsRow, map[string]bool, error) {
	signals := []db.ListAlertSignalsRow{}
	uncertain := map[string]bool{}
	if !cfg.ModelAlerts && cfg.QuotaThreshold == 0 {
		return signals, uncertain, nil
	}
	if observer == nil {
		return nil, nil, ErrCapacityUnavailable
	}
	pools, err := observer(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	current := map[string]db.ListAlertSignalsRow{}
	for _, pool := range pools {
		if pool.Unknown {
			uncertain["model_unavailable:"+strconv.FormatInt(pool.ID, 10)+":*"] = true
			uncertain["quota_low:*"] = true
		}
		for _, model := range pool.Models {
			subject := strconv.FormatInt(pool.ID, 10) + ":" + model.Model
			key := "model_unavailable:" + subject
			if cfg.ModelAlerts {
				if model.State == "unknown" {
					uncertain[key] = true
				} else if model.State == "unavailable" {
					structural := false
					transient := false
					for _, reason := range model.Reasons {
						if reason.Code == "account_busy" {
							transient = true
						} else {
							structural = true
						}
					}
					// Saturation is short-lived admission pressure, not a model outage.
					if structural && !transient {
						current[key] = db.ListAlertSignalsRow{Kind: "model_unavailable", Subject: subject}
					}
				}
			}
			if cfg.QuotaThreshold > 0 {
				for _, account := range model.Accounts {
					if account.Reason == "account_disabled" || account.Reason == "account_reauthorization_required" || account.Reason == "provider_disabled" {
						continue
					}
					key := "quota_low:" + account.ID
					if account.QuotaState == "stale" || account.QuotaState == "unknown" || (account.QuotaUsedPercent == nil && account.QuotaState != "unsupported") {
						uncertain[key] = true
						continue
					}
					if account.QuotaUsedPercent != nil && *account.QuotaUsedPercent >= float64(cfg.QuotaThreshold) {
						current[key] = db.ListAlertSignalsRow{Kind: "quota_low", Subject: account.ID}
					}
				}
			}
		}
	}
	for _, signal := range current {
		signals = append(signals, signal)
	}
	return signals, uncertain, nil
}
