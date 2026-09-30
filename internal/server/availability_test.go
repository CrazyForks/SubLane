package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/apikey"
)

func TestModelAvailabilityEndpointsProtectIdentityAndKeyLifecycle(t *testing.T) {
	f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("inspection contacted upstream") })
	h := f.server.Config.Handler
	login := func(name, password string) *http.Cookie {
		response := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": name, "password": password}, nil)
		if response.Code != 200 {
			t.Fatal(response.Code)
		}
		return response.Result().Cookies()[0]
	}
	member := login("member-test", "member pass 42")
	owner := login("owner-test", "owner pass 42")
	path := fmt.Sprintf("/api/keys/%d/models/availability?model=synthetic-model", f.keyID)
	if response := request(h, "GET", path, "", nil, nil); response.Code != 401 {
		t.Fatal(response.Code)
	}
	response := request(h, "GET", path, "", nil, member)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"state":"available"`) || strings.Contains(response.Body.String(), `"accounts"`) || strings.Contains(response.Body.String(), "synthetic-upstream") {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := request(h, "GET", path, "", nil, owner); response.Code != 404 {
		t.Fatal("another owner's key", response.Code)
	}
	poolPath := "/api/groups/1/models/availability?model=synthetic-model"
	if response := request(h, "GET", poolPath, "", nil, member); response.Code != 403 {
		t.Fatal(response.Code)
	}
	response = request(h, "GET", poolPath, "", nil, owner)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"accounts"`) {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := request(h, "GET", "/api/groups/1/models/availability?model=", "", nil, owner); response.Code != 400 {
		t.Fatal("invalid model", response.Code)
	}
	if _, err := f.keys.Update(context.Background(), f.userID, f.keyID, apikey.UpdateInput{Name: "Synthetic client", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if response := request(h, "GET", path, "", nil, member); response.Code != 409 {
		t.Fatal("disabled key", response.Code)
	}
}
