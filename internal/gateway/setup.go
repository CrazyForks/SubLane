package gateway

import (
	"context"

	"github.com/murongg/SubLane/internal/storage/db"
)

type Setup struct {
	Stage                string `json:"stage"`
	HasSuccessfulRequest bool   `json:"has_successful_request"`
}

// SetupProgress reads local state only. A model catalog or valid key is not evidence of successful inference.
func (s *Service) SetupProgress(ctx context.Context, userID int64, administrator, usableKey bool) (Setup, error) {
	first, err := s.queries.GetSetupMembership(ctx, db.GetSetupMembershipParams{TenantID: s.tenantID, UserID: userID})
	if err != nil {
		return Setup{}, err
	}
	result := Setup{Stage: "access", HasSuccessfulRequest: first > 0}
	if administrator {
		counts, err := s.queries.GetSetupAccounts(ctx, s.tenantID)
		if err != nil {
			return Setup{}, err
		}
		if counts.Enabled == 0 {
			result.Stage = "account"
			return result, nil
		}
		if counts.Verified == 0 {
			result.Stage = "verify"
			return result, nil
		}
		result.Stage = "pool"
	}
	status, err := s.queries.GroupConnectionStatus(ctx, db.GroupConnectionStatusParams{TenantID: s.tenantID, UserID: userID})
	if err != nil {
		return Setup{}, err
	}
	if status != "ready" {
		return result, nil
	}
	result.Stage = "key"
	if usableKey {
		result.Stage = "client"
	}
	if usableKey && result.HasSuccessfulRequest {
		result.Stage = "complete"
	}
	return result, nil
}
