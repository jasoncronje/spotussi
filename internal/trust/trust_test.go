package trust

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParse(t *testing.T) {
	p, err := Parse("10.0.0.0/8, 192.168.1.2 ::1,\t172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 4 {
		t.Fatalf("got %v", p)
	}
	for _, bad := range []string{"10.0.0.0/33", "nope", "1.2.3"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	if p, err := Parse(""); err != nil || len(p) != 0 {
		t.Errorf("empty: %v %v", p, err)
	}
}

func TestForwardedHeadersOnlyFromTrustedPeers(t *testing.T) {
	p, _ := Parse("10.0.0.0/8")
	req := func(remote string) *http.Request {
		r := httptest.NewRequest("GET", "http://internal:8080/x", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Host", "stats.example.com")
		r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.7")
		return r
	}

	trusted := req("10.1.2.3:5555")
	if got := p.Origin(trusted); got != "https://stats.example.com" {
		t.Errorf("trusted origin = %q", got)
	}
	if got := p.ClientIP(trusted).String(); got != "203.0.113.9" {
		t.Errorf("trusted client ip = %q", got)
	}

	untrusted := req("198.51.100.4:5555")
	if got := p.Origin(untrusted); got != "http://internal:8080" {
		t.Errorf("untrusted origin = %q", got)
	}
	if got := p.ClientIP(untrusted).String(); got != "198.51.100.4" {
		t.Errorf("untrusted client ip = %q", got)
	}

	// No trusted proxies at all: headers never count.
	if got := Proxies(nil).Origin(trusted); got != "http://internal:8080" {
		t.Errorf("no proxies origin = %q", got)
	}

	// Malformed forwarded host falls back to Host.
	bad := req("10.1.2.3:5555")
	bad.Header.Set("X-Forwarded-Host", "evil.com/path")
	if got := p.Host(bad); got != "internal:8080" {
		t.Errorf("bad forwarded host = %q", got)
	}

	// Direct TLS.
	tlsReq := req("198.51.100.4:5555")
	tlsReq.TLS = &tls.ConnectionState{}
	if got := p.Scheme(tlsReq); got != "https" {
		t.Errorf("tls scheme = %q", got)
	}
}

func TestValidHost(t *testing.T) {
	for h, want := range map[string]bool{
		"example.com": true, "example.com:8443": true, "[::1]:80": true, "127.0.0.1": true,
		"": false, "a b": false, "user@host": false, "host/path": false, "host?x": false, ":80": false,
	} {
		if got := ValidHost(h); got != want {
			t.Errorf("ValidHost(%q) = %v", h, got)
		}
	}
}
