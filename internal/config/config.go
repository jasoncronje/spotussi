// Package config loads process configuration from the environment.
//
// Every variable is optional (D15): a container started with no environment
// at all must come up and serve the setup flow. Settings that the owner
// changes at runtime (play rule, timezone, ...) live in the database, not here.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"spotussi/internal/trust"
)

// AuthMode selects how the instance authenticates browser users (D13).
type AuthMode string

const (
	AuthPassword AuthMode = "password"
	AuthProxy    AuthMode = "proxy"
	AuthNone     AuthMode = "none"
)

// Config is the environment-derived configuration.
type Config struct {
	// Port is the HTTP listen port (PORT, default 8080).
	Port int
	// Host is the listen address (HOST, default all interfaces). Dev
	// tooling sets 127.0.0.1 so local servers are not exposed on the LAN.
	Host string
	// DataDir holds the SQLite database and encryption key (DATA_DIR,
	// default ./data; the container image sets /data).
	DataDir string
	// BaseURL overrides the externally visible URL used for OAuth redirects
	// (BASE_URL, optional, D12). Empty means "derive from the request".
	BaseURL string
	// AuthMode selects browser authentication (AUTH_MODE, default
	// password, D13).
	AuthMode AuthMode
	// AuthModeSet reports whether AUTH_MODE was given explicitly (mock
	// mode defaults to none unless it was).
	AuthModeSet bool
	// DataDirSet reports whether DATA_DIR was given explicitly (mock mode
	// uses a scratch directory unless it was).
	DataDirSet bool
	// ProxyAuthHeader carries the user name in proxy mode
	// (AUTH_PROXY_HEADER, default Remote-User).
	ProxyAuthHeader string
	// TrustedProxies are the reverse proxies whose forwarding headers
	// (X-Forwarded-*, and the auth header in proxy mode) are believed
	// (TRUSTED_PROXIES: comma-separated IPs or CIDRs). Empty trusts none.
	TrustedProxies trust.Proxies

	// Spotify request budget (D9; zero means the built-in default):
	// SpotifyRate is requests per second (SPOTIFY_RATE), SpotifyBurst the
	// bucket size (SPOTIFY_BURST), SpotifyLiveReserve the share of the
	// bucket reserved for live capture (SPOTIFY_LIVE_RESERVE, 0.01-0.9).
	SpotifyRate        float64
	SpotifyBurst       int
	SpotifyLiveReserve float64
	// SpotifyLoopbackPort overrides the port in the plain-HTTP redirect
	// URI http://127.0.0.1:<port>/oauth/spotify/callback
	// (SPOTIFY_LOOPBACK_PORT; default: the port in the request's Host,
	// else PORT).
	SpotifyLoopbackPort int

	// Live capture (D2). CaptureEnabled (CAPTURE_ENABLED, default true)
	// runs the poller and backstop. Intervals (Go durations, zero means
	// default): CAPTURE_PLAYING_INTERVAL while playing,
	// CAPTURE_IDLE_INTERVAL when paused or idle, CAPTURE_RECENT_INTERVAL
	// for the recently-played backstop.
	CaptureEnabled         bool
	CapturePlayingInterval time.Duration
	CaptureIdleInterval    time.Duration
	CaptureRecentInterval  time.Duration

	// HydrateEnabled (HYDRATE_ENABLED, default true) runs catalog
	// hydration (M4, D9). Turning it off keeps a development instance
	// from spending the shared Spotify quota; the catalog then only
	// reports what is stored.
	HydrateEnabled bool
	// HydrateSpotifyLookups (HYDRATE_SPOTIFY_LOOKUPS, default true) lets
	// hydration look entities up on Spotify. Off, the worker makes no
	// Spotify catalog calls at all (tracks, artists, albums, playlists):
	// seeding from live capture and the fallback sources still run, and
	// Spotify queue entries stay queued without being charged (D27).
	HydrateSpotifyLookups bool
	// Fallback metadata sources (D27), each on by default and used only
	// while hydration runs: Spotify oEmbed (METADATA_OEMBED), Deezer
	// (METADATA_DEEZER), ListenBrainz (METADATA_LISTENBRAINZ, D37),
	// MusicBrainz (METADATA_MUSICBRAINZ).
	// MetadataContact (METADATA_CONTACT) is appended to the User-Agent
	// they get (a URL or address of the instance's operator; unset by
	// default).
	MetadataOEmbed       bool
	MetadataDeezer       bool
	MetadataListenBrainz bool
	MetadataMusicBrainz  bool
	MetadataContact      string

	// LibrarySyncEnabled (LIBRARY_SYNC_ENABLED, default true) syncs saved
	// tracks, saved albums and playlists from Spotify (D36).
	// LibrarySyncInterval (LIBRARY_SYNC_INTERVAL, default 1h) is the time
	// between quick checks; every list is walked in full once a day.
	LibrarySyncEnabled  bool
	LibrarySyncInterval time.Duration

	// ImportMaxBytes caps one uploaded export (IMPORT_MAX_BYTES, default
	// 2 GiB); larger uploads get 413 (D23).
	ImportMaxBytes int64
}

// DefaultImportMaxBytes is used when IMPORT_MAX_BYTES is unset.
const DefaultImportMaxBytes = 2 << 30

// DefaultPort is used when PORT is unset.
const DefaultPort = 8080

// DefaultProxyAuthHeader is the forward-auth header used by Authelia,
// Authentik and most other forward-auth providers.
const DefaultProxyAuthHeader = "Remote-User"

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup reads the configuration through lookup, which has the
// signature of os.LookupEnv. Tests pass a map-backed function.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key string) string {
		v, _ := lookup(key)
		return strings.TrimSpace(v)
	}
	cfg := Config{Port: DefaultPort, DataDir: "./data", AuthMode: AuthPassword, ProxyAuthHeader: DefaultProxyAuthHeader,
		CaptureEnabled: true, HydrateEnabled: true, LibrarySyncEnabled: true, HydrateSpotifyLookups: true, ImportMaxBytes: DefaultImportMaxBytes,
		MetadataOEmbed: true, MetadataDeezer: true, MetadataListenBrainz: true, MetadataMusicBrainz: true}
	var errs []error
	number := func(key string, lo, hi float64) float64 {
		v := get(key)
		if v == "" {
			return 0
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < lo || f > hi {
			errs = append(errs, fmt.Errorf("%s must be a number between %g and %g, got %q", key, lo, hi, v))
			return 0
		}
		return f
	}
	duration := func(key string, lo, hi time.Duration) time.Duration {
		v := get(key)
		if v == "" {
			return 0
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < lo || d > hi {
			errs = append(errs, fmt.Errorf("%s must be a duration between %s and %s (e.g. 15s, 2m), got %q", key, lo, hi, v))
			return 0
		}
		return d
	}
	cfg.SpotifyRate = number("SPOTIFY_RATE", 0.05, 50)
	cfg.SpotifyBurst = int(number("SPOTIFY_BURST", 1, 500))
	cfg.SpotifyLiveReserve = number("SPOTIFY_LIVE_RESERVE", 0.01, 0.9)
	cfg.SpotifyLoopbackPort = int(number("SPOTIFY_LOOPBACK_PORT", 1, 65535))
	cfg.CapturePlayingInterval = duration("CAPTURE_PLAYING_INTERVAL", 3*time.Second, 2*time.Minute)
	cfg.CaptureIdleInterval = duration("CAPTURE_IDLE_INTERVAL", 10*time.Second, 30*time.Minute)
	cfg.CaptureRecentInterval = duration("CAPTURE_RECENT_INTERVAL", time.Minute, 6*time.Hour)
	if v := get("CAPTURE_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("CAPTURE_ENABLED must be true or false, got %q", v))
		} else {
			cfg.CaptureEnabled = b
		}
	}

	if v := get("HYDRATE_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("HYDRATE_ENABLED must be true or false, got %q", v))
		} else {
			cfg.HydrateEnabled = b
		}
	}
	cfg.LibrarySyncInterval = duration("LIBRARY_SYNC_INTERVAL", 10*time.Minute, 24*time.Hour)
	for key, dst := range map[string]*bool{"LIBRARY_SYNC_ENABLED": &cfg.LibrarySyncEnabled, "HYDRATE_SPOTIFY_LOOKUPS": &cfg.HydrateSpotifyLookups, "METADATA_OEMBED": &cfg.MetadataOEmbed, "METADATA_DEEZER": &cfg.MetadataDeezer,
		"METADATA_LISTENBRAINZ": &cfg.MetadataListenBrainz, "METADATA_MUSICBRAINZ": &cfg.MetadataMusicBrainz} {
		if v := get(key); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s must be true or false, got %q", key, v))
			} else {
				*dst = b
			}
		}
	}
	cfg.MetadataContact = get("METADATA_CONTACT")
	if v := get("IMPORT_MAX_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1<<20 || n > 1<<40 {
			errs = append(errs, fmt.Errorf("IMPORT_MAX_BYTES must be a byte count between 1 MiB and 1 TiB, got %q", v))
		} else {
			cfg.ImportMaxBytes = n
		}
	}
	if v := get("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			errs = append(errs, fmt.Errorf("PORT must be an integer between 1 and 65535, got %q", v))
		} else {
			cfg.Port = p
		}
	}
	if v := get("HOST"); v != "" {
		if strings.ContainsAny(v, "/: ") && net.ParseIP(v) == nil {
			errs = append(errs, fmt.Errorf("HOST must be an IP address or hostname, got %q", v))
		} else {
			cfg.Host = v
		}
	}
	if v := get("DATA_DIR"); v != "" {
		cfg.DataDir, cfg.DataDirSet = v, true
	}
	if v := get("BASE_URL"); v != "" {
		u, err := url.Parse(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("BASE_URL: %w", err))
		case u.Scheme != "http" && u.Scheme != "https":
			errs = append(errs, fmt.Errorf("BASE_URL must start with http:// or https://, got %q", v))
		case u.Host == "":
			errs = append(errs, fmt.Errorf("BASE_URL must include a host, got %q", v))
		case u.RawQuery != "" || u.Fragment != "":
			errs = append(errs, fmt.Errorf("BASE_URL must not contain a query or fragment, got %q", v))
		default:
			cfg.BaseURL = strings.TrimRight(v, "/")
		}
	}
	if v := get("AUTH_MODE"); v != "" {
		switch m := AuthMode(strings.ToLower(v)); m {
		case AuthPassword, AuthProxy, AuthNone:
			cfg.AuthMode, cfg.AuthModeSet = m, true
		default:
			errs = append(errs, fmt.Errorf("AUTH_MODE must be one of password, proxy, none; got %q", v))
		}
	}
	if v := get("AUTH_PROXY_HEADER"); v != "" {
		if strings.ContainsAny(v, " :\t\r\n") {
			errs = append(errs, fmt.Errorf("AUTH_PROXY_HEADER must be a header name, got %q", v))
		} else {
			cfg.ProxyAuthHeader = v
		}
	}
	if v := get("TRUSTED_PROXIES"); v != "" {
		p, err := trust.Parse(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXIES: %w", err))
		}
		cfg.TrustedProxies = p
	}
	if cfg.AuthMode == AuthProxy && len(cfg.TrustedProxies) == 0 {
		errs = append(errs, errors.New("AUTH_MODE=proxy requires TRUSTED_PROXIES (the reverse proxy's IP or network)"))
	}
	return cfg, errors.Join(errs...)
}
