package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

const syntheticLimitMetadata = `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED","domain":"cloudcode-pa.googleapis.com","metadata":{"model":"synthetic-model"}}]}}`

func TestModelLimitMetadataSurvivesSDK(t *testing.T) {
	var paths []string
	c := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(syntheticLimitMetadata))}, nil
	}))
	defer c.Close()
	credential := accounts.Credential{Provider: "antigravity", AccessToken: "synthetic-access", ExpiresAt: time.Now().Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"project_id": json.RawMessage(`"synthetic-project"`)}}
	stream, err := c.Gemini(context.Background(), credential, []byte(`{"model":"synthetic-model","contents":[{"parts":[{"text":"Synthetic input"}]}]}`), nil, false)
	if stream != nil {
		stream.Body.Close()
	}
	var rejected *UpstreamError
	if !errors.As(err, &rejected) || rejected.LimitedModel != "synthetic-model" || rejected.RetryAfter != "120" {
		t.Fatalf("SDK lost model scope: %v; synthetic request paths: %v", err, paths)
	}
}

func TestModelLimitTransportPreservesBodyAndScope(t *testing.T) {
	transport := &engineTransport{provider: "antigravity", requestedModel: "synthetic-model", base: usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(syntheticLimitMetadata))}, nil
	})}
	req, _ := http.NewRequest(http.MethodPost, "https://cloudcode-pa.googleapis.com/v1internal:generateContent", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(raw) != syntheticLimitMetadata || transport.limitedModel(429) != "synthetic-model" {
		t.Fatal("error metadata observation changed body or lost scope", err, transport.limitedModel(429))
	}
}

func TestModelLimitMetadataBoundsAndUnknownScope(t *testing.T) {
	for _, raw := range []string{`null`, strings.ReplaceAll(syntheticLimitMetadata, "synthetic-model", "synthetic-other"), strings.ReplaceAll(syntheticLimitMetadata, "RATE_LIMIT_EXCEEDED", "QUOTA_EXHAUSTED"), strings.ReplaceAll(syntheticLimitMetadata, "cloudcode-pa.googleapis.com", "unknown.example.test"), strings.Repeat(" ", maxLimitMetadata) + syntheticLimitMetadata} {
		if antigravityLimitedModel([]byte(raw), "synthetic-model") != "" {
			t.Fatal("uncertain or oversized metadata weakened the account limit")
		}
	}
	captured := false
	body := &limitMetadataBody{ReadCloser: io.NopCloser(strings.NewReader(syntheticLimitMetadata)), complete: func([]byte) { captured = true }}
	if _, err := body.Read(make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	body.Close()
	if captured {
		t.Fatal("partial metadata produced a model limit")
	}
}
