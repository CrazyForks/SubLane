package gateway

import (
	"context"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/storage/db"
)

type RequestRecord struct {
	ID              int64  `json:"id"`
	UserID          int64  `json:"user_id"`
	KeyID           int64  `json:"key_id"`
	GroupID         int64  `json:"group_id"`
	AccountID       string `json:"account_id"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	Transport       string `json:"transport"`
	Operation       string `json:"operation"`
	StartedAt       int64  `json:"started_at"`
	DurationMs      int64  `json:"duration_ms"`
	Outcome         string `json:"outcome"`
	ErrorCode       string `json:"error_code"`
	UpstreamStatus  *int64 `json:"upstream_status"`
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	CachedTokens    *int64 `json:"cached_tokens"`
	RequestID       string `json:"request_id"`
	FirstTokenMs    *int64 `json:"first_token_ms"`
	ReasoningEffort string `json:"reasoning_effort"`
	Username        string `json:"username"`
	KeyName         string `json:"key_name"`
	GroupName       string `json:"group_name"`
	AccountName     string `json:"account_name"`
}
type RequestPage struct {
	Requests   []RequestRecord `json:"requests"`
	NextCursor int64           `json:"next_cursor"`
}

type RequestFilter struct {
	Cursor                               int64
	AccountID, Outcome, Model, RequestID string
	From, Until, KeyID, MemberID         int64
}

func (s *Service) Requests(ctx context.Context, filter RequestFilter) (RequestPage, error) {
	return s.requests(ctx, 0, filter)
}
func (s *Service) UserRequests(ctx context.Context, userID int64, filter RequestFilter) (RequestPage, error) {
	if userID <= 0 {
		return RequestPage{}, accounts.ErrInput
	}
	// Caller-controlled operator filters can never replace the session's ownership scope.
	filter.MemberID = 0
	filter.AccountID = ""
	return s.requests(ctx, userID, filter)
}
func (s *Service) requests(ctx context.Context, userID int64, f RequestFilter) (RequestPage, error) {
	page := RequestPage{Requests: []RequestRecord{}}
	if f.Cursor < 0 || len(f.AccountID) > 64 || f.KeyID < 0 || f.MemberID < 0 || f.From < 0 || f.Until < 0 || (f.Until != 0 && f.From >= f.Until) || (f.Model != "" && !safeModel.MatchString(f.Model)) || (f.RequestID != "" && !safeRequestID.MatchString(f.RequestID)) {
		return page, accounts.ErrInput
	}
	switch f.Outcome {
	case "", "success", "incomplete", "error", "canceled", "rejected":
	default:
		return page, accounts.ErrInput
	}
	since := s.now().Add(-7 * 24 * time.Hour).Unix()
	rows, err := s.queries.ListRequests(ctx, db.ListRequestsParams{TenantID: s.tenantID, UserID: userID, Cursor: f.Cursor, AccountID: f.AccountID, Outcome: f.Outcome, Since: max(since, f.From), UntilTime: f.Until, MemberID: f.MemberID, KeyID: f.KeyID, Model: f.Model, RequestID: f.RequestID})
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		record := RequestRecord(row)
		if userID != 0 {
			record.AccountID, record.AccountName = "", ""
		}
		page.Requests = append(page.Requests, record)
	}
	if len(page.Requests) > 50 {
		page.Requests = page.Requests[:50]
		page.NextCursor = page.Requests[49].ID
	}
	return page, nil
}

type RequestCaller struct {
	UserID   int64  `json:"user_id"`
	KeyID    int64  `json:"key_id"`
	Username string `json:"username"`
	KeyName  string `json:"key_name"`
}

func (s *Service) RequestCallers(ctx context.Context, userID int64) ([]RequestCaller, error) {
	rows, err := s.queries.ListRequestCallers(ctx, db.ListRequestCallersParams{TenantID: s.tenantID, UserID: userID, Since: s.now().Add(-7 * 24 * time.Hour).Unix()})
	if err != nil {
		return nil, err
	}
	result := make([]RequestCaller, 0, len(rows))
	for _, row := range rows {
		result = append(result, RequestCaller(row))
	}
	return result, nil
}
