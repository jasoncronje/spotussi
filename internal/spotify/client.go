package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/keyring"
)

const (
	clientIDSetting = "spotify_client_id"
	refreshPurpose  = "spotify/refresh_token"
	// authRequestTTL bounds how long the owner has to approve access.
	authRequestTTL = 15 * time.Minute
	// maxPendingRequests caps stored in-flight authorizations.
	maxPendingRequests = 20
	// expirySlack refreshes access tokens this long before they expire.
	expirySlack = time.Minute
)

// State is the connection state reported in system status.
type State string

const (
	StateNotConfigured     State = "not_configured"
	StateConnected         State = "connected"
	StateReconnectRequired State = "reconnect_required"
)

// Errors.
var (
	ErrNoClientID        = errors.New("set the Spotify Client ID first")
	ErrInvalidClientID   = errors.New("the Client ID must be the 32-character ID shown in the Spotify app dashboard")
	ErrInvalidState      = errors.New("this authorization link is unknown, already used or expired; start connecting again")
	ErrNotConnected      = errors.New("Spotify is not connected")
	ErrReconnectRequired = errors.New("Spotify access was revoked or expired; reconnect Spotify")
	ErrInvalidReturnPath = errors.New("return_to must be a path on this site")
)

// DeniedError means the owner did not approve access (or Spotify refused
// the request); Code is Spotify's error parameter, e.g. access_denied.
type DeniedError struct{ Code string }

func (e *DeniedError) Error() string {
	if e.Code == "access_denied" {
		return "access was not approved on Spotify"
	}
	return "Spotify returned an error: " + e.Code
}

// APIError is an unexpected response from Spotify.
type APIError struct {
	Status  int
	Code    string // OAuth error code or API error message
	Message string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("Spotify responded %d", e.Status)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// Options configure a Client.
type Options struct {
	DB      *db.DB
	Keyring *keyring.Keyring
	// AccountsURL and APIURL default to Spotify's (tests use fakes).
	AccountsURL, APIURL string
	HTTPClient          *http.Client
	Now                 func() time.Time
	// Limiter is the shared request budget (D9); defaults to one built
	// from DefaultLimiterConfig.
	Limiter *Limiter
}

// Client owns the Spotify connection: Client ID, OAuth, tokens and API
// requests. It is safe for concurrent use and shared by the HTTP layer and
// (from M2) the capture workers.
type Client struct {
	db          *db.DB
	keys        *keyring.Keyring
	accountsURL string
	apiURL      string
	http        *http.Client
	now         func() time.Time
	limiter     *Limiter

	// tok guards the cached access token and serialises refreshes and
	// connection writes: a single-flight lock that waiters can abandon
	// via their context.
	tok    chan struct{}
	access string
	expiry time.Time

	hookMu    sync.Mutex
	onConnect []func()
}

// OnConnect registers f to run after every successful connection, e.g.
// to wake live capture instead of waiting out its idle interval. f must
// not block.
func (c *Client) OnConnect(f func()) {
	c.hookMu.Lock()
	defer c.hookMu.Unlock()
	c.onConnect = append(c.onConnect, f)
}

func (c *Client) connected() {
	c.hookMu.Lock()
	hooks := append([]func(){}, c.onConnect...)
	c.hookMu.Unlock()
	for _, f := range hooks {
		f()
	}
}

// New builds a Client.
func New(o Options) *Client {
	c := &Client{db: o.DB, keys: o.Keyring, accountsURL: o.AccountsURL, apiURL: o.APIURL,
		http: o.HTTPClient, now: o.Now, limiter: o.Limiter, tok: make(chan struct{}, 1)}
	if c.accountsURL == "" {
		c.accountsURL = DefaultAccountsURL
	}
	if c.apiURL == "" {
		c.apiURL = DefaultAPIURL
	}
	c.accountsURL = strings.TrimRight(c.accountsURL, "/")
	c.apiURL = strings.TrimRight(c.apiURL, "/")
	if c.http == nil {
		c.http = &http.Client{Timeout: 20 * time.Second}
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.limiter == nil {
		c.limiter = NewLimiter(DefaultLimiterConfig, nil)
	}
	return c
}

func (c *Client) lock(ctx context.Context) error {
	select {
	case c.tok <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) unlock() { <-c.tok }

// ---------- Client ID ----------

// ClientID returns the stored Client ID ("" when unset).
func (c *Client) ClientID(ctx context.Context) (string, error) {
	var id string
	_, err := c.db.GetSetting(ctx, clientIDSetting, &id)
	return id, err
}

// SetClientID stores the owner's Client ID. Refresh tokens are bound to
// the app that issued them, so changing it drops an existing connection.
func (c *Client) SetClientID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if !ValidClientID(id) {
		return ErrInvalidClientID
	}
	if err := c.lock(ctx); err != nil {
		return err
	}
	defer c.unlock()
	if err := c.db.SetSetting(ctx, clientIDSetting, id); err != nil {
		return err
	}
	res, err := c.db.Write.ExecContext(ctx, `DELETE FROM spotify_connection WHERE client_id <> ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("spotify: client ID changed; previous connection removed")
		c.access, c.expiry = "", time.Time{}
	}
	return nil
}

// ---------- status ----------

// Status describes the connection.
type Status struct {
	State       State
	ClientID    string
	AccountID   string
	AccountName string // may be empty (Spotify display names are optional)
	ConnectedAt int64
	Scopes      []string
	LastError   string
}

// Status reads the connection state.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	id, err := c.ClientID(ctx)
	if err != nil {
		return nil, err
	}
	st := &Status{State: StateNotConfigured, ClientID: id}
	var name, lastErr sql.NullString
	var scopes string
	var reconnect bool
	err = c.db.Read.QueryRowContext(ctx, `SELECT account_id, account_name, scopes, connected_at, needs_reconnect, last_error
		FROM spotify_connection WHERE id = 1`).Scan(&st.AccountID, &name, &scopes, &st.ConnectedAt, &reconnect, &lastErr)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.State = StateConnected
	if reconnect {
		st.State = StateReconnectRequired
	}
	st.AccountName, st.LastError = name.String, lastErr.String
	st.Scopes = strings.Fields(scopes)
	return st, nil
}

// ---------- authorization ----------

// AuthRequest is a started authorization.
type AuthRequest struct {
	URL         string
	RedirectURI string
	ExpiresAt   time.Time
}

// BeginAuth starts the PKCE flow: it stores a fresh state and verifier and
// returns the Spotify consent URL. redirectURI must be the one Spotify
// will redirect to (see Redirect); returnTo is where the browser goes
// after a direct callback.
func (c *Client) BeginAuth(ctx context.Context, redirectURI, returnTo string) (*AuthRequest, error) {
	if returnTo == "" {
		returnTo = "/"
	}
	if !SafeReturnPath(returnTo) {
		return nil, ErrInvalidReturnPath
	}
	clientID, err := c.ClientID(ctx)
	if err != nil {
		return nil, err
	}
	if clientID == "" {
		return nil, ErrNoClientID
	}
	now := c.now()
	verifier, challenge := pkce()
	state := randomState()
	req := &AuthRequest{URL: authorizeURL(c.accountsURL, clientID, redirectURI, state, challenge),
		RedirectURI: redirectURI, ExpiresAt: now.Add(authRequestTTL)}

	tx, err := c.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM spotify_auth_requests WHERE expires_at <= ?`, now.UnixMilli()); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM spotify_auth_requests WHERE state NOT IN
		(SELECT state FROM spotify_auth_requests ORDER BY created_at DESC LIMIT ?)`, maxPendingRequests-1); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO spotify_auth_requests
		(state, verifier, client_id, redirect_uri, return_to, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		state, verifier, clientID, redirectURI, returnTo, now.UnixMilli(), req.ExpiresAt.UnixMilli()); err != nil {
		return nil, err
	}
	return req, tx.Commit()
}

// Complete finishes an authorization from the callback parameters (direct
// callback or pasted URL, same path): it consumes the state, exchanges the
// code with the stored PKCE verifier, fetches the account (GET /me), and
// stores the encrypted refresh token. It returns the stored return path
// whenever the state was valid, even on error.
func (c *Client) Complete(ctx context.Context, p CallbackParams) (returnTo string, err error) {
	if p.State == "" {
		return "", ErrInvalidState
	}
	var verifier, clientID, redirectURI string
	var expires int64
	err = c.db.Write.QueryRowContext(ctx, `DELETE FROM spotify_auth_requests WHERE state = ?
		RETURNING verifier, client_id, redirect_uri, return_to, expires_at`, p.State).
		Scan(&verifier, &clientID, &redirectURI, &returnTo, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidState
	}
	if err != nil {
		return "", err
	}
	if c.now().UnixMilli() >= expires {
		return returnTo, ErrInvalidState
	}
	if p.Error != "" {
		return returnTo, &DeniedError{Code: p.Error}
	}
	if p.Code == "" {
		return returnTo, ErrInvalidCallbackURL
	}

	tr, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {p.Code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		return returnTo, err
	}
	if tr.RefreshToken == "" {
		return returnTo, &APIError{Status: http.StatusOK, Message: "token response has no refresh_token"}
	}
	me, err := c.me(ctx, tr.AccessToken)
	if err != nil {
		return returnTo, fmt.Errorf("read the connected account: %w", err)
	}
	if err := c.lock(ctx); err != nil {
		return returnTo, err
	}
	defer c.unlock()
	now := c.now()
	var name *string
	if me.DisplayName != "" {
		name = &me.DisplayName
	}
	_, err = c.db.Write.ExecContext(ctx, `INSERT OR REPLACE INTO spotify_connection
		(id, client_id, account_id, account_name, scopes, refresh_token, connected_at, refreshed_at, needs_reconnect, last_error)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, 0, NULL)`,
		clientID, me.ID, name, tr.Scope, c.keys.Seal(refreshPurpose, []byte(tr.RefreshToken)), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return returnTo, fmt.Errorf("store connection: %w", err)
	}
	c.access, c.expiry = tr.AccessToken, now.Add(time.Duration(tr.ExpiresIn)*time.Second)
	slog.Info("spotify: connected", "account", me.ID, "scopes", tr.Scope)
	c.connected()
	return returnTo, nil
}

// Disconnect forgets the connection (the Client ID is kept). Spotify has
// no token revocation endpoint; the owner can remove access under their
// Spotify account's "Manage apps".
func (c *Client) Disconnect(ctx context.Context) error {
	if err := c.lock(ctx); err != nil {
		return err
	}
	defer c.unlock()
	c.access, c.expiry = "", time.Time{}
	_, err := c.db.Write.ExecContext(ctx, `DELETE FROM spotify_connection`)
	return err
}

// ---------- tokens ----------

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type oauthError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func (c *Client) tokenRequest(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.accountsURL+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		var oe oauthError
		_ = json.Unmarshal(body, &oe)
		return nil, &APIError{Status: resp.StatusCode, Code: oe.Error, Message: oe.Description}
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return nil, &APIError{Status: resp.StatusCode, Message: "malformed token response"}
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	return &tr, nil
}

// AccessToken returns a valid access token, refreshing it if needed. Only
// one refresh runs at a time; concurrent callers wait for it and reuse its
// result. A rotated refresh token is persisted before the new access
// token is handed out. It returns ErrNotConnected or ErrReconnectRequired
// when there is no usable connection.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if err := c.lock(ctx); err != nil {
		return "", err
	}
	defer c.unlock()
	if c.access != "" && c.now().Add(expirySlack).Before(c.expiry) {
		return c.access, nil
	}
	return c.refreshLocked(ctx)
}

// invalidate drops the cached access token if it is still tok (after
// Spotify rejected it), so the next AccessToken call refreshes.
func (c *Client) invalidate(ctx context.Context, tok string) {
	if c.lock(ctx) != nil {
		return
	}
	if c.access == tok {
		c.access, c.expiry = "", time.Time{}
	}
	c.unlock()
}

func (c *Client) refreshLocked(ctx context.Context) (string, error) {
	var clientID string
	var sealed []byte
	var reconnect bool
	err := c.db.Read.QueryRowContext(ctx, `SELECT client_id, refresh_token, needs_reconnect FROM spotify_connection WHERE id = 1`).
		Scan(&clientID, &sealed, &reconnect)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotConnected
	}
	if err != nil {
		return "", err
	}
	if reconnect {
		return "", ErrReconnectRequired
	}
	refresh, err := c.keys.Open(refreshPurpose, sealed)
	if err != nil {
		return "", c.markReconnect(ctx, "the stored refresh token cannot be decrypted (was the key file replaced?)")
	}
	tr, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {string(refresh)},
		"client_id":     {clientID},
	})
	var ae *APIError
	if errors.As(err, &ae) && (ae.Code == "invalid_grant" || ae.Code == "invalid_client") {
		return "", c.markReconnect(ctx, ae.Error())
	}
	if err != nil {
		return "", err
	}
	now := c.now()
	if tr.RefreshToken != "" && tr.RefreshToken != string(refresh) {
		_, err = c.db.Write.ExecContext(ctx, `UPDATE spotify_connection SET refresh_token = ?, refreshed_at = ?, last_error = NULL WHERE id = 1`,
			c.keys.Seal(refreshPurpose, []byte(tr.RefreshToken)), now.UnixMilli())
	} else {
		_, err = c.db.Write.ExecContext(ctx, `UPDATE spotify_connection SET refreshed_at = ?, last_error = NULL WHERE id = 1`, now.UnixMilli())
	}
	if err != nil {
		// Spotify may already have invalidated the old refresh token, so
		// failing to store the new one is serious: do not use the access
		// token as if all were well.
		return "", fmt.Errorf("persist rotated refresh token: %w", err)
	}
	c.access, c.expiry = tr.AccessToken, now.Add(time.Duration(tr.ExpiresIn)*time.Second)
	return c.access, nil
}

func (c *Client) markReconnect(ctx context.Context, reason string) error {
	slog.Warn("spotify: reconnect required", "reason", reason)
	c.access, c.expiry = "", time.Time{}
	if _, err := c.db.Write.ExecContext(ctx, `UPDATE spotify_connection SET needs_reconnect = 1, last_error = ? WHERE id = 1`, reason); err != nil {
		return err
	}
	return ErrReconnectRequired
}

// ---------- Web API ----------

// Do sends an authenticated Web API request for path (relative to the API
// base, e.g. "/me/player"). If Spotify rejects the access token (401) the
// token is refreshed and the request retried once. The caller closes the
// response body. Requests wait on the shared limiter in ctx's lane
// (WithPriority); a 429 is returned as *RateLimitedError.
func (c *Client) Do(ctx context.Context, method, path string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		tok, err := c.AccessToken(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := c.send(ctx, method, path, tok)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			c.invalidate(ctx, tok)
			continue
		}
		return resp, nil
	}
}

// send makes one Web API request through the shared limiter (in ctx's
// lane, see WithPriority). A 429 pauses the limiter (or, for
// QUOTA_EXCEEDED, the path's quota bucket) and is returned as a
// *RateLimitedError with the body consumed; while a bucket is paused its
// requests fail the same way without being sent.
func (c *Client) send(ctx context.Context, method, path, token string) (*http.Response, error) {
	bucket := Bucket(path)
	if until := c.limiter.QuotaBlockedUntil(bucket); !until.IsZero() {
		d := until.Sub(c.now())
		return nil, &RateLimitedError{RetryAfter: d, Backoff: d, QuotaExceeded: true, Reason: "QUOTA_EXCEEDED",
			Bucket: bucket, NotSent: true}
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify %s %s: %w", method, path, err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		e := &RateLimitedError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), c.now()), Reason: rateLimitReason(body)}
		e.QuotaExceeded = strings.EqualFold(e.Reason, "QUOTA_EXCEEDED")
		if e.QuotaExceeded {
			e.Bucket = bucket
			e.Backoff = c.limiter.QuotaExceeded(bucket, e.RetryAfter, e.Reason)
		} else {
			e.Backoff = c.limiter.Throttled(e.RetryAfter, false, e.Reason)
		}
		return nil, e
	}
	if resp.StatusCode < 400 {
		c.limiter.Success()
	}
	return resp, nil
}

// LimiterStats reports the shared rate limiter's metrics.
func (c *Client) LimiterStats() LimiterStats { return c.limiter.Stats() }

// User is the connected account (GET /me).
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Me returns the connected account.
func (c *Client) Me(ctx context.Context) (*User, error) {
	resp, err := c.Do(ctx, http.MethodGet, "/me")
	if err != nil {
		return nil, err
	}
	return decodeUser(resp)
}

func (c *Client) me(ctx context.Context, token string) (*User, error) {
	resp, err := c.send(ctx, http.MethodGet, "/me", token)
	if err != nil {
		return nil, err
	}
	return decodeUser(resp)
}

func decodeUser(resp *http.Response) (*User, error) {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp.StatusCode, body)
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil || u.ID == "" {
		return nil, &APIError{Status: resp.StatusCode, Message: "malformed /me response"}
	}
	return &u, nil
}

// apiError decodes a Web API error body ({"error":{"status","message"}}).
func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	ae := &APIError{Status: status, Message: e.Error.Message}
	if status == http.StatusForbidden && ae.Message == "" {
		ae.Message = "forbidden"
	}
	return ae
}
