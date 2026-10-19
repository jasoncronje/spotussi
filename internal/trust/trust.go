// Package trust decides which request metadata to believe.
//
// Forwarding headers (X-Forwarded-For, X-Forwarded-Proto, X-Forwarded-Host)
// and the proxy-auth identity header (D13) are only honoured when the TCP
// peer is one of the configured trusted proxies (TRUSTED_PROXIES). For any
// other peer they are ignored, so a client cannot spoof its address, the
// scheme, the host, or its identity by sending headers.
package trust

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// Proxies is the set of trusted reverse-proxy networks.
type Proxies []netip.Prefix

// Parse reads a comma- or space-separated list of IP addresses and CIDR
// prefixes ("10.0.0.0/8, 192.168.1.2, ::1").
func Parse(s string) (Proxies, error) {
	var out Proxies
	for f := range strings.FieldsFuncSeq(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if strings.Contains(f, "/") {
			p, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q", f)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address %q", f)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// Contains reports whether a is inside a trusted network.
func (p Proxies) Contains(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	for _, pre := range p {
		if pre.Contains(a) {
			return true
		}
	}
	return false
}

// PeerIP is the address of the TCP peer (r.RemoteAddr).
func PeerIP(r *http.Request) netip.Addr {
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr().Unmap()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

// FromProxy reports whether the request arrived from a trusted proxy.
func (p Proxies) FromProxy(r *http.Request) bool {
	return len(p) > 0 && p.Contains(PeerIP(r))
}

// ClientIP is the address of the client: the peer, or, when the peer is a
// trusted proxy, the right-most X-Forwarded-For entry that is not itself a
// trusted proxy (entries left of that could be forged by the client).
func (p Proxies) ClientIP(r *http.Request) netip.Addr {
	peer := PeerIP(r)
	if !p.Contains(peer) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		if a = a.Unmap(); !p.Contains(a) {
			return a
		}
	}
	return peer
}

// Scheme is "https" when the request reached us over TLS, or a trusted
// proxy says the client used HTTPS; otherwise "http".
func (p Proxies) Scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p.FromProxy(r) {
		if v := firstValue(r.Header.Get("X-Forwarded-Proto")); strings.EqualFold(v, "https") {
			return "https"
		}
	}
	return "http"
}

// Host is the host[:port] the client addressed: X-Forwarded-Host from a
// trusted proxy, otherwise the Host header. Malformed values fall back to
// the Host header.
func (p Proxies) Host(r *http.Request) string {
	if p.FromProxy(r) {
		if v := firstValue(r.Header.Get("X-Forwarded-Host")); v != "" && ValidHost(v) {
			return v
		}
	}
	return r.Host
}

// Origin is scheme://host as seen by the client.
func (p Proxies) Origin(r *http.Request) string {
	return p.Scheme(r) + "://" + p.Host(r)
}

// ValidHost reports whether h is a bare host or host:port with nothing
// else (no userinfo, path, query or whitespace).
func ValidHost(h string) bool {
	if h == "" || strings.ContainsAny(h, "/\\@?# \t\r\n") {
		return false
	}
	u, err := url.Parse("http://" + h)
	return err == nil && u.Host == h && u.Hostname() != ""
}

func firstValue(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return strings.TrimSpace(first)
}
