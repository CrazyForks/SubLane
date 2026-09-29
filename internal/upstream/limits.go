package upstream

import (
	"encoding/json"
	"io"
	"sync"
)

const maxLimitMetadata = 16 << 10

// Narrow only explicit structured Antigravity model limits. A generic 429,
// unknown reason/domain or mismatched model retains account-wide protection.
func antigravityLimitedModel(raw []byte, model string) string {
	if model == "" || len(model) > 128 || len(raw) > maxLimitMetadata {
		return ""
	}
	var value struct {
		Error struct {
			Code    int    `json:"code"`
			Status  string `json:"status"`
			Details []struct {
				Type     string            `json:"@type"`
				Reason   string            `json:"reason"`
				Domain   string            `json:"domain"`
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Error.Code != 429 || value.Error.Status != "RESOURCE_EXHAUSTED" || len(value.Error.Details) > 16 {
		return ""
	}
	match := ""
	infos := 0
	for _, detail := range value.Error.Details {
		if detail.Type == "type.googleapis.com/google.rpc.ErrorInfo" {
			infos++
			if infos > 1 {
				return ""
			}
		}
		if detail.Type == "type.googleapis.com/google.rpc.ErrorInfo" && detail.Reason == "RATE_LIMIT_EXCEEDED" && detail.Domain == "cloudcode-pa.googleapis.com" && detail.Metadata["model"] == model && detail.Metadata["quotaScope"] != "account" {
			match = model
		}
	}
	return match
}

// Observe a small error body as the SDK reads it, preserving its original bytes
// and deadlines. Oversized or incomplete metadata cannot weaken cooldown scope.
type limitMetadataBody struct {
	io.ReadCloser
	mu       sync.Mutex
	closed   bool
	raw      []byte
	overflow bool
	complete func([]byte)
}

func (b *limitMetadataBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return n, err
	}
	if !b.overflow {
		if len(b.raw)+n > maxLimitMetadata {
			b.overflow = true
			b.raw = nil
		} else {
			b.raw = append(b.raw, p[:n]...)
		}
	}
	if err == io.EOF && b.complete != nil {
		if !b.overflow {
			b.complete(b.raw)
		}
		b.complete = nil
		b.raw = nil
	}
	return n, err
}
func (b *limitMetadataBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.raw = nil
	b.complete = nil
	b.mu.Unlock()
	return b.ReadCloser.Close()
}
