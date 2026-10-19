package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProxyOptions configures RunProxy.
type ProxyOptions struct {
	// URL is the server's base URL (https://stats.example.com) or the full
	// /mcp endpoint.
	URL   string
	Token string
	In    io.Reader
	Out   io.Writer
	// Log receives diagnostics (stderr in the CLI); nil discards them.
	Log io.Writer
	// Client defaults to one with a 60 s timeout.
	Client *http.Client
}

// Endpoint returns the /mcp URL for a base URL.
func Endpoint(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, Path) {
		return base
	}
	return base + Path
}

// maxMessageBytes bounds one stdio message (the server accepts 1 MiB).
const maxMessageBytes = 4 << 20

// RunProxy bridges a stdio MCP client to the Streamable HTTP endpoint: each
// newline-delimited JSON-RPC message from In is POSTed to /mcp with the API
// token, and every JSON-RPC message in the response is written to Out, one
// per line. Messages are forwarded concurrently (responses carry their
// request's id). HTTP and network failures become JSON-RPC errors for the
// request that caused them, so the client shows why. It returns when In
// reaches EOF and every forwarded request has finished.
func RunProxy(ctx context.Context, o ProxyOptions) error {
	if o.URL == "" {
		return errors.New("no server URL")
	}
	if o.Token == "" {
		return errors.New("no API token")
	}
	p := &proxy{o: o, endpoint: Endpoint(o.URL), client: o.Client}
	if p.client == nil {
		p.client = &http.Client{Timeout: 60 * time.Second}
	}
	if p.o.Log == nil {
		p.o.Log = io.Discard
	}
	sc := bufio.NewScanner(o.In)
	sc.Buffer(make([]byte, 64<<10), maxMessageBytes)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		msg := bytes.Clone(line)
		var head struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(msg, &head); err != nil {
			fmt.Fprintf(p.o.Log, "spotussi mcp: ignoring a message that is not a JSON object: %v\n", err)
			continue
		}
		// initialize goes first and alone: its answer sets the protocol
		// version header that later requests carry.
		if head.Method == "initialize" {
			wg.Wait()
			p.forward(ctx, msg, head.ID, true)
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			p.forward(ctx, msg, head.ID, false)
		}()
	}
	wg.Wait()
	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	return nil
}

type proxy struct {
	o        ProxyOptions
	endpoint string
	client   *http.Client

	mu        sync.Mutex // guards out, version, sessionID
	version   string
	sessionID string
}

func (p *proxy) headers() (version, session string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.version, p.sessionID
}

// write sends one message to the client as a single line.
func (p *proxy) write(msg []byte) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, msg); err != nil {
		fmt.Fprintf(p.o.Log, "spotussi mcp: dropping an invalid message from the server: %v\n", err)
		return
	}
	buf.WriteByte('\n')
	p.mu.Lock()
	defer p.mu.Unlock()
	p.o.Out.Write(buf.Bytes())
}

// fail answers a request (one with an id) with a JSON-RPC error.
func (p *proxy) fail(id json.RawMessage, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(p.o.Log, "spotussi mcp: %s\n", msg)
	if len(id) == 0 || string(id) == "null" {
		return
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": -32000, "message": "spotussi: " + msg}})
	p.write(body)
}

func (p *proxy) forward(ctx context.Context, msg, id json.RawMessage, initialize bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(msg))
	if err != nil {
		p.fail(id, "bad server URL %q: %v", p.endpoint, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+p.o.Token)
	if v, s := p.headers(); !initialize {
		if v != "" {
			req.Header.Set("MCP-Protocol-Version", v)
		}
		if s != "" {
			req.Header.Set("Mcp-Session-Id", s)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.fail(id, "cannot reach %s: %v", p.endpoint, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		p.fail(id, "%s answered %s: %s", p.endpoint, resp.Status, errorMessage(resp.Body))
		return
	}
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		p.mu.Lock()
		p.sessionID = s
		p.mu.Unlock()
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch {
	case resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent:
		return
	case ct == "text/event-stream":
		err = readEvents(resp.Body, func(data []byte) {
			if initialize {
				p.noteVersion(data)
			}
			p.write(data)
		})
	case ct == "application/json":
		var body []byte
		body, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err == nil && len(bytes.TrimSpace(body)) > 0 {
			if initialize {
				p.noteVersion(body)
			}
			p.write(body)
		}
	default:
		p.fail(id, "%s answered with unexpected content type %q (is the URL a spotussi server?)", p.endpoint, ct)
		return
	}
	if err != nil {
		p.fail(id, "reading the response: %v", err)
	}
}

// noteVersion records the protocol version negotiated by initialize.
func (p *proxy) noteVersion(body []byte) {
	var r struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &r) == nil && r.Result.ProtocolVersion != "" {
		p.mu.Lock()
		p.version = r.Result.ProtocolVersion
		p.mu.Unlock()
	}
}

// errorMessage extracts the message of a spotussi JSON error body, or the
// start of any other body.
func errorMessage(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, 4<<10))
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// readEvents calls fn with the data of each server-sent event.
func readEvents(r io.Reader, fn func([]byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	var data bytes.Buffer
	flush := func() {
		if data.Len() > 0 {
			fn(bytes.Clone(data.Bytes()))
			data.Reset()
		}
	}
	for sc.Scan() {
		line := sc.Bytes()
		switch {
		case len(line) == 0:
			flush()
		case bytes.HasPrefix(line, []byte("data:")):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.Write(bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" ")))
		}
	}
	flush()
	return sc.Err()
}
