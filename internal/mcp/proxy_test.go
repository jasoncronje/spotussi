package mcp

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/config"
)

// stdioClient runs RunProxy in-process on pipes and connects an SDK client
// to its stdio side.
func stdioClient(t *testing.T, url, token string) (*sdk.ClientSession, *strings.Builder, error) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	logs := &strings.Builder{}
	done := make(chan error, 1)
	go func() {
		done <- RunProxy(context.Background(), ProxyOptions{URL: url, Token: token, In: inR, Out: outW, Log: logs})
		outW.Close()
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("proxy: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("proxy did not exit at EOF")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, logs, err
}

func TestStdioProxyEndToEnd(t *testing.T) {
	a := newAuth(t, config.AuthPassword)
	srv := newHTTPServer(t, a)
	_, secret, err := a.CreateToken(context.Background(), "stdio")
	if err != nil {
		t.Fatal(err)
	}
	cs, _, err := stdioClient(t, srv.URL+"/", secret)
	if err != nil {
		t.Fatalf("connect through the proxy: %v", err)
	}
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 8 {
		t.Fatalf("list tools: %v (%d tools)", err, len(tools.Tools))
	}
	// Concurrent calls come back to the right callers.
	errs := make(chan error, 8)
	for _, tool := range tools.Tools {
		go func() {
			args := map[string]any{}
			switch tool.Name {
			case "get_top":
				args["type"] = "albums"
			case "get_entity":
				args["kind"], args["id"] = "artist", "missing"
			case "search_catalog":
				args["query"] = "the"
			}
			res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: tool.Name, Arguments: args})
			switch {
			case err != nil:
				errs <- err
			case tool.Name == "get_entity" && !res.IsError:
				errs <- io.ErrUnexpectedEOF
			case tool.Name != "get_entity" && res.IsError:
				errs <- io.ErrNoProgress
			default:
				errs <- nil
			}
		}()
	}
	for range tools.Tools {
		if err := <-errs; err != nil {
			t.Errorf("call through the proxy: %v", err)
		}
	}
}

func TestStdioProxyReportsAuthErrors(t *testing.T) {
	srv := newHTTPServer(t, newAuth(t, config.AuthPassword))
	_, logs, err := stdioClient(t, srv.URL, "spt_revoked")
	if err == nil || !strings.Contains(err.Error(), "invalid or revoked API token") {
		t.Errorf("connect with a bad token: %v", err)
	}
	if !strings.Contains(logs.String(), "401") {
		t.Errorf("proxy log: %q", logs.String())
	}
}

func TestStdioProxyUnreachable(t *testing.T) {
	_, _, err := stdioClient(t, "http://127.0.0.1:1", "spt_x")
	if err == nil || !strings.Contains(err.Error(), "cannot reach http://127.0.0.1:1/mcp") {
		t.Errorf("unreachable server: %v", err)
	}
}

func TestEndpoint(t *testing.T) {
	for in, want := range map[string]string{
		"https://s.example.com":      "https://s.example.com/mcp",
		"https://s.example.com/":     "https://s.example.com/mcp",
		"https://s.example.com/mcp":  "https://s.example.com/mcp",
		"https://s.example.com/mcp/": "https://s.example.com/mcp",
	} {
		if got := Endpoint(in); got != want {
			t.Errorf("Endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
