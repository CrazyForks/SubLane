package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestConnectionReportsPersonalSetupWithoutAccountDetails(t *testing.T) {
	f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("setup called upstream") })
	h := f.server.Config.Handler
	login := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": "owner-test", "password": "owner pass 42"}, nil)
	cookie := login.Result().Cookies()[0]
	got := request(h, "GET", "/api/connection", "", nil, cookie)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"stage":"key"`) || strings.Contains(got.Body.String(), "account_id") {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(h, "GET", "/api/connection", "", nil, nil); got.Code != 401 {
		t.Fatal(got.Code)
	}
}
