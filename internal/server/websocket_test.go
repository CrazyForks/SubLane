package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebsocketConfiguredRequestBodyLimit(t *testing.T) {
	const limit = 1024
	var calls atomic.Int32
	fixture := newForwardingFixture(t, "codex", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"output\":[]}}\n\n")
	}, false, limit)
	for _, size := range []int{limit - 1, limit, limit + 1} {
		func() {
			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			raw := `{"type":"response.create","model":"synthetic-model","input":[]}`
			raw += strings.Repeat(" ", size-len(raw))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
				t.Fatal(err)
			}
			for {
				_, data, err := conn.ReadMessage()
				if size > limit {
					if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
						t.Fatalf("oversized websocket message: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var event struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(data, &event); err != nil {
					t.Fatal(err)
				}
				if event.Type == "error" {
					t.Fatal("request within websocket limit rejected")
				}
				if event.Type == "response.completed" {
					return
				}
			}
		}()
	}
	if calls.Load() != 2 {
		t.Fatalf("oversized websocket message reached upstream: calls=%d", calls.Load())
	}
}

func TestWebsocketPrewarmAndPerTurnKeyRevocation(t *testing.T) {
	var calls atomic.Int32
	fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"output\":[]}}\n\n")
	})
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("websocket upgrade: %d %v", status, err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	read := func(want string) map[string]any {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if json.Unmarshal(data, &event) != nil {
				t.Fatal("invalid websocket event")
			}
			if event["type"] == want {
				return event
			}
			if event["type"] == "error" {
				t.Fatal("unexpected websocket error")
			}
		}
	}
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "model": "synthetic-model", "input": []any{}, "generate": false}); err != nil {
		t.Fatal(err)
	}
	warmed := read("response.completed")
	if warmed["request_id"] != nil {
		t.Fatal("prewarm fabricated a recorded request ID")
	}
	if calls.Load() != 0 {
		t.Fatal("prewarm made a generation request")
	}
	previous := warmed["response"].(map[string]any)["id"]
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "previous_response_id": previous, "input": []any{}}); err != nil {
		t.Fatal(err)
	}
	completed := read("response.completed")
	id, ok := completed["request_id"].(string)
	if !ok || !strings.HasPrefix(id, "req_") {
		t.Fatal("missing turn correlation", completed)
	}
	if calls.Load() != 1 {
		t.Fatal("generation was not forwarded")
	}
	if _, err := fixture.keys.Revoke(context.Background(), fixture.userID, fixture.keyID); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "response.create", "previous_response_id": "resp_test", "input": []any{}}); err != nil {
		t.Fatal(err)
	}
	rejected := read("error")
	if rejected["request_id"] != nil {
		t.Fatal("unrecorded turn reused previous correlation")
	}
	detail := rejected["error"].(map[string]any)
	if detail["code"] != "invalid_api_key" || calls.Load() != 1 {
		t.Fatal("revoked key started another turn")
	}
}
