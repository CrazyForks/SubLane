package server

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProviderRoutesUseBuiltInExecutors(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity", "xai"} {
		t.Run(provider, func(t *testing.T) {
			fixture := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if provider == "claude" {
					if r.URL.Path != "/v1/messages" {
						t.Errorf("wrong Claude endpoint %s", r.URL.Path)
					}
					io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_synthetic\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"synthetic-model\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Synthetic response\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				} else if provider == "xai" {
					if r.URL.Path != "/v1/responses" || r.Header.Get("X-XAI-Token-Auth") != "xai-grok-cli" {
						t.Error("wrong Grok subscription request")
					}
					io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Synthetic response\",\"output_index\":0,\"content_index\":0}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic-response\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"synthetic-model\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Synthetic response\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
				} else {
					if !strings.Contains(r.URL.Path, "streamGenerateContent") {
						t.Errorf("wrong Antigravity endpoint %s", r.URL.Path)
					}
					io.WriteString(w, "data: {\"response\":{\"responseId\":\"synthetic-response\",\"modelVersion\":\"synthetic-model\",\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Synthetic response\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1,\"totalTokenCount\":2}}}\n\n")
				}
			})

			for _, test := range []struct{ path, body, want string }{
				{"/v1/responses", `{"model":"` + provider + `/synthetic-model","input":"synthetic legacy prompt"}`, `"output"`},
				{"/v1/responses", `{"model":"synthetic-model","input":"synthetic prompt"}`, `"output"`},
				{"/v1/responses", `{"model":"synthetic-model","input":"synthetic prompt","stream":true}`, "response.completed"},
				{"/v1/chat/completions", `{"model":"synthetic-model","messages":[{"role":"user","content":"synthetic prompt"}]}`, `"choices"`},
				{"/v1/chat/completions", `{"model":"synthetic-model","messages":[{"role":"user","content":"synthetic prompt"}],"stream":true}`, "[DONE]"},
			} {
				req, _ := http.NewRequest("POST", fixture.server.URL+test.path, strings.NewReader(test.body))
				req.Header.Set("Authorization", "Bearer "+fixture.secret)
				req.Header.Set("Content-Type", "application/json")
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != 200 || !strings.Contains(string(body), "Synthetic response") || !strings.Contains(string(body), test.want) {
					t.Fatalf("%s failed: %d %s", test.path, response.StatusCode, body)
				}
			}

			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(fixture.server.URL, "http://", "ws://", 1)+"/v1/responses", http.Header{"Authorization": {"Bearer " + fixture.secret}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			read := func(want string) []byte {
				for {
					_, data, err := conn.ReadMessage()
					if err != nil {
						t.Fatal(err)
					}
					var event struct {
						Type string `json:"type"`
					}
					if json.Unmarshal(data, &event) != nil {
						t.Fatal("invalid websocket response")
					}
					if event.Type == want {
						return data
					}
					if event.Type == "error" {
						t.Fatal("unexpected websocket error", string(data))
					}
				}
			}
			turn := map[string]any{"type": "response.create", "model": "synthetic-model", "input": "synthetic prompt"}
			if err := conn.WriteJSON(turn); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(read("response.completed")), "Synthetic response") {
				t.Fatal("missing websocket output")
			}
			if _, err := fixture.keys.Revoke(context.Background(), fixture.userID, fixture.keyID); err != nil {
				t.Fatal(err)
			}
			if err := conn.WriteJSON(turn); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(read("error")), "invalid_api_key") {
				t.Fatal("revoked key accepted on provider websocket")
			}

		})
	}
}
