package api

import (
	"net/http"
	"net/url"
	"testing"

	"spotussi/internal/config"
)

// An instance opened on a loopback IP over plain HTTP completes the
// connection through the normal callback, port included (D21).
func TestSpotifyLoopbackDirectCallback(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthNone})
	const host = "127.0.0.1:8080"
	owner := func(method, path string, body any) map[string]any {
		rec := s.do(req{method: method, path: path, body: body, host: host})
		expect(t, rec, 200, "")
		return decode(t, rec)
	}
	owner("PUT", "/api/v1/spotify/client-id", map[string]string{"client_id": testClientID})
	authz := owner("POST", "/api/v1/spotify/authorize", map[string]string{"return_to": "/setup"})
	cb := authz["callback"].(map[string]any)
	if cb["mode"] != "direct" || cb["redirect_uri"] != "http://127.0.0.1:8080/oauth/spotify/callback" {
		t.Fatalf("callback: %v", cb)
	}
	back, err := s.fake.Approve(authz["authorize_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(back)
	if u.Scheme+"://"+u.Host+u.Path != "http://127.0.0.1:8080/oauth/spotify/callback" {
		t.Fatalf("spotify redirected to %s", back)
	}
	rec := s.do(req{method: "GET", path: u.RequestURI(), host: host})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/setup?spotify=connected" {
		t.Fatalf("callback: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if st := owner("GET", "/api/v1/spotify/connection", nil); st["state"] != "connected" {
		t.Fatalf("connection: %v", st)
	}
	// The token exchange used the same redirect URI (Spotify requires it).
	if got := s.fake.Seen["token:authorization_code"].Get("redirect_uri"); got != "http://127.0.0.1:8080/oauth/spotify/callback" {
		t.Fatalf("token exchange redirect_uri %q", got)
	}
}
