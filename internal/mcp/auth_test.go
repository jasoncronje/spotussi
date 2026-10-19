package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/auth"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
)

func newAuth(t *testing.T, mode config.AuthMode) *auth.Service {
	t.Helper()
	d, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	keys, _ := keyring.New(bytes.Repeat([]byte{2}, 32))
	return auth.New(auth.Options{DB: d, Keyring: keys, Mode: mode, Hash: auth.Params{MemoryKiB: 64, Time: 1, Threads: 1}})
}

// newHTTPServer serves Routes on an httptest server, as `serve` mounts them.
func newHTTPServer(t *testing.T, a *auth.Service) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range Routes(Options{Backend: mockBackend(), Auth: a, Version: "test", Mode: "mock"}) {
		mux.Handle(pattern, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// post sends a raw initialize request with extra headers.
func post(t *testing.T, url string, hdr ...string) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url+Path, strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

// httpClient connects the SDK's Streamable HTTP client with a token.
func httpClient(t *testing.T, url, token string) (*sdk.ClientSession, error) {
	t.Helper()
	tr := &sdk.StreamableClientTransport{Endpoint: url + Path, DisableStandaloneSSE: true, MaxRetries: -1,
		HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}}}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(context.Background(), tr, nil)
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, err
}

func errorCode(body string) string {
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	json.Unmarshal([]byte(body), &e)
	return e.Error.Code
}

func TestAuthPasswordMode(t *testing.T) {
	a := newAuth(t, config.AuthPassword)
	srv := newHTTPServer(t, a)
	ctx := context.Background()

	if code, hdr, body := post(t, srv.URL); code != 401 || errorCode(body) != "unauthorized" || hdr.Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("no token: %d %s %v", code, body, hdr)
	}
	if code, hdr, body := post(t, srv.URL, "Authorization", "Bearer spt_nope"); code != 401 || errorCode(body) != "invalid_token" ||
		!strings.Contains(hdr.Get("WWW-Authenticate"), "invalid_token") {
		t.Errorf("bad token: %d %s", code, body)
	}
	if code, _, body := post(t, srv.URL, "Authorization", "Basic b3duZXI6eA=="); code != 401 || errorCode(body) != "invalid_token" {
		t.Errorf("basic auth: %d %s", code, body)
	}

	// A signed-in browser session is not an MCP credential.
	sess, err := a.Setup(ctx, netip.MustParseAddr("127.0.0.1"), "correct horse battery", a.SetupToken())
	if err != nil {
		t.Fatal(err)
	}
	if code, _, body := post(t, srv.URL, "Cookie", auth.CookieName+"="+sess.Token); code != 401 || errorCode(body) != "token_required" {
		t.Errorf("session cookie: %d %s", code, body)
	}

	tok, secret, err := a.CreateToken(ctx, "claude")
	if err != nil {
		t.Fatal(err)
	}
	cs, err := httpClient(t, srv.URL, secret)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "get_top", Arguments: map[string]any{"type": "artists", "limit": 2}})
	if err != nil || res.IsError {
		t.Fatalf("tool call over HTTP: %v %+v", err, res)
	}
	if !strings.Contains(res.Content[0].(*sdk.TextContent).Text, `"rank":1`) {
		t.Errorf("result %s", res.Content[0].(*sdk.TextContent).Text)
	}

	// Revoked: the very next request fails, on a fresh or an open client.
	if err := a.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if code, _, body := post(t, srv.URL, "Authorization", "Bearer "+secret); code != 401 || errorCode(body) != "invalid_token" {
		t.Errorf("revoked token: %d %s", code, body)
	}
	if _, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "get_status"}); err == nil {
		t.Errorf("revoked token: call on an open client succeeded")
	}
	if _, err := httpClient(t, srv.URL, secret); err == nil {
		t.Errorf("revoked token: connect succeeded")
	}
}

func TestAuthNoneMode(t *testing.T) {
	a := newAuth(t, config.AuthNone)
	srv := newHTTPServer(t, a)
	if code, _, body := post(t, srv.URL); code != 200 || !strings.Contains(body, `"serverInfo"`) {
		t.Errorf("AUTH_MODE=none without a token: %d %s", code, body)
	}
	// A token that is sent must still be valid.
	if code, _, body := post(t, srv.URL, "Authorization", "Bearer spt_nope"); code != 401 || errorCode(body) != "invalid_token" {
		t.Errorf("AUTH_MODE=none with a bad token: %d %s", code, body)
	}
}

func TestOAuthDiscoveryNotFound(t *testing.T) {
	srv := newHTTPServer(t, newAuth(t, config.AuthPassword))
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 404 || resp.Header.Get("Content-Type") != "application/json" || errorCode(string(body)) != "not_found" {
			t.Errorf("%s: %d %s", p, resp.StatusCode, body)
		}
	}
}
