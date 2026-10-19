package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"spotussi/internal/trust"
)

func TestWarnUntrustedProxy(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	send := func(h http.Handler, remote string, forwarded bool) {
		r := httptest.NewRequest("GET", "/api/v1/health", nil)
		r.RemoteAddr = remote
		if forwarded {
			r.Header.Set("X-Forwarded-Proto", "https")
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	trusted, _ := trust.Parse("10.0.0.0/8")

	h := warnUntrustedProxy(trusted, "", ok)
	send(h, "203.0.113.5:1000", false) // no forwarding headers
	send(h, "10.1.2.3:1000", true)     // trusted proxy
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning: %s", buf.String())
	}
	send(h, "172.18.0.1:1000", true)
	send(h, "172.18.0.1:1000", true)
	out := buf.String()
	if strings.Count(out, "TRUSTED_PROXIES=172.18.0.1") != 1 || !strings.Contains(out, "plain-HTTP redirect URI") {
		t.Fatalf("warning: %s", out)
	}
	buf.Reset()
	send(warnUntrustedProxy(nil, "https://stats.example.com", ok), "172.18.0.1:1000", true)
	if out := buf.String(); !strings.Contains(out, "Secure") || strings.Contains(out, "redirect URI") {
		t.Fatalf("warning with BASE_URL: %s", out)
	}
}
