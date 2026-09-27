package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/gateway"
)

func TestGatewayDiagnosticIDIgnoresCallerAndMatchesRecord(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[]}}\n\n")
	})
	for _, model := range []string{"synthetic-model", "not-available"} {
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"`+model+`","input":"synthetic"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.secret)
		req.Header.Set("X-Request-ID", "untrusted-correlation")
		w := httptest.NewRecorder()
		f.server.Config.Handler.ServeHTTP(w, req)
		id := w.Header().Get("X-Request-ID")
		if id == "" || id == "untrusted-correlation" {
			t.Fatal("invalid correlation header", id)
		}
		page, err := f.forwarding.UserRequests(context.Background(), f.userID, gateway.RequestFilter{RequestID: id})
		if err != nil || len(page.Requests) != 1 || page.Requests[0].Model != model {
			t.Fatal("response cannot be correlated", err)
		}
	}
}

func TestQuotaErrorHasBoundedRetryAndSafeReason(t *testing.T) {
	w := httptest.NewRecorder()
	gatewayError(w, &gateway.QuotaError{RetryAfter: 90})
	if w.Code != 429 || w.Header().Get("Retry-After") != "90" || !strings.Contains(w.Body.String(), "quota_exhausted") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestAllocationRiskLimitExplainsTemporaryAdmissionPause(t *testing.T) {
	w := httptest.NewRecorder()
	gatewayError(w, allocations.ErrRisk)
	if w.Code != 429 || !strings.Contains(w.Body.String(), "allocation_risk_limit") || w.Header().Get("Retry-After") != "" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
}

func TestWebSocketCorrelationPreservesResponsePayload(t *testing.T) {
	original := []byte(`{"type":"response.created","response":{"id":"synthetic-response"}}`)
	raw := correlateEvent(original, "req_synthetic")
	var value struct {
		RequestID string `json:"request_id"`
		Response  struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &value) != nil || value.RequestID != "req_synthetic" || value.Response.ID != "synthetic-response" {
		t.Fatal("invalid correlated event", string(raw))
	}
}
