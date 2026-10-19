package api

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"spotussi/internal/trust"
)

// warnUntrustedProxy logs once when requests carry forwarding headers from
// a peer outside TRUSTED_PROXIES. That is almost always a reverse proxy
// the operator forgot to trust: its headers are ignored (correctly), so
// the setup wizard offers the plain-HTTP loopback redirect URI and session
// cookies are not marked Secure, with nothing else pointing at the cause
// (found in the M8 fresh-install walkthrough).
func warnUntrustedProxy(proxies trust.Proxies, baseURL string, next http.Handler) http.Handler {
	var warned atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !warned.Load() && (r.Header.Get("X-Forwarded-Proto") != "" || r.Header.Get("X-Forwarded-For") != "") {
			if peer := trust.PeerIP(r); peer.IsValid() && !proxies.Contains(peer) && warned.CompareAndSwap(false, true) {
				msg := "requests carry X-Forwarded-* headers from a peer that is not in TRUSTED_PROXIES, so they are ignored: " +
					"session cookies are not marked Secure"
				if baseURL == "" {
					msg += " and the Spotify setup offers the plain-HTTP redirect URI instead of https://<host>/oauth/spotify/callback"
				}
				slog.Warn(msg+". If this is your reverse proxy, set TRUSTED_PROXIES to its address or network",
					"peer", peer.String(), "suggestion", "TRUSTED_PROXIES="+peer.String())
			}
		}
		next.ServeHTTP(w, r)
	})
}
