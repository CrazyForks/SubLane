package gateway

import (
	"log/slog"

	"github.com/murongg/SubLane/internal/logging"
)

func (e *observation) log(level slog.Level, message, code string) {
	logger := logging.From(e.ctx)
	if !logger.Enabled(e.ctx, level) {
		return
	}
	r := e.record
	attrs := []slog.Attr{
		slog.String("request_id", r.RequestID), slog.Int64("tenant_id", e.service.tenantID),
		slog.Int64("user_id", r.UserID), slog.Int64("key_id", r.KeyID), slog.Int64("group_id", r.GroupID),
		slog.String("account_id", r.AccountID), slog.String("provider", r.Provider), slog.String("model", r.Model),
		slog.String("transport", r.Transport), slog.String("operation", r.Operation),
		slog.String("outcome", r.Outcome), slog.String("error_code", code), slog.Int64("duration_ms", r.DurationMs),
	}
	if r.UpstreamStatus != nil {
		attrs = append(attrs, slog.Int64("upstream_status", *r.UpstreamStatus))
	}
	if r.FirstTokenMs != nil {
		attrs = append(attrs, slog.Int64("first_token_ms", *r.FirstTokenMs))
	}
	logger.LogAttrs(e.ctx, level, message, attrs...)
}
