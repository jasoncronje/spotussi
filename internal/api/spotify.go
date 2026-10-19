package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"spotussi/internal/spotify"
	"spotussi/internal/trust"
)

// Spotify connection endpoints (D12). The setup wizard reads
// GET /spotify/connection for the callback mode and the exact redirect URI
// to register, stores the Client ID, then starts the flow with
// POST /spotify/authorize. Over HTTPS, or plain HTTP on a loopback IP,
// Spotify redirects the browser to /oauth/spotify/callback; over other
// plain-HTTP hosts the owner pastes the loopback URL into
// POST /spotify/paste-back. Both finish through spotify.Complete.

type SpotifyCallback struct {
	Mode        string `json:"mode"`
	RedirectURI string `json:"redirect_uri"`
}

type SpotifyConnection struct {
	State       string          `json:"state"`
	ClientID    *string         `json:"client_id"`
	AccountID   *string         `json:"account_id"`
	AccountName *string         `json:"account_name"`
	ConnectedAt *int64          `json:"connected_at"`
	Scopes      []string        `json:"scopes"`
	LastError   *string         `json:"last_error"`
	Callback    SpotifyCallback `json:"callback"`
}

type SpotifyClientIDInput struct {
	ClientID string `json:"client_id"`
}

type SpotifyAuthorizeInput struct {
	ReturnTo string `json:"return_to,omitempty"`
}

type SpotifyAuthorization struct {
	AuthorizeURL string          `json:"authorize_url"`
	Callback     SpotifyCallback `json:"callback"`
	ExpiresAt    int64           `json:"expires_at"`
}

type SpotifyPasteBackInput struct {
	URL string `json:"url"`
}

// externalOrigin is the instance's URL as the browser sees it: BASE_URL,
// else scheme://host from the request (forwarded headers only from
// trusted proxies).
func externalOrigin(r *http.Request, baseURL string, proxies trust.Proxies) string {
	if baseURL != "" {
		return baseURL
	}
	return proxies.Origin(r)
}

func spotifyError(err error) error {
	var de *spotify.DeniedError
	var ae *spotify.APIError
	switch {
	case errors.Is(err, spotify.ErrInvalidClientID), errors.Is(err, spotify.ErrInvalidReturnPath):
		return errorf(http.StatusBadRequest, "bad_request", "%s", err)
	case errors.Is(err, spotify.ErrNoClientID):
		return errorf(http.StatusConflict, "client_id_required", "%s", err)
	case errors.Is(err, spotify.ErrInvalidCallbackURL):
		return errorf(http.StatusBadRequest, "invalid_callback_url", "%s", err)
	case errors.Is(err, spotify.ErrInvalidState):
		return errorf(http.StatusBadRequest, "invalid_state", "%s", err)
	case errors.As(err, &de):
		return errorf(http.StatusBadRequest, "spotify_denied", "%s", err)
	case errors.As(err, &ae):
		return errorf(http.StatusBadGateway, "spotify_error", "%s", err)
	}
	return err
}

func spotifyConnection(r *http.Request, c *spotify.Client, origin string, lp spotify.LoopbackPort) (*SpotifyConnection, error) {
	st, err := c.Status(r.Context())
	if err != nil {
		return nil, err
	}
	mode, uri := spotify.Redirect(origin, lp)
	out := &SpotifyConnection{State: string(st.State), Scopes: []string{},
		Callback: SpotifyCallback{Mode: string(mode), RedirectURI: uri}}
	if st.ClientID != "" {
		out.ClientID = &st.ClientID
	}
	if st.State != spotify.StateNotConfigured {
		out.AccountID, out.ConnectedAt = &st.AccountID, &st.ConnectedAt
		if st.AccountName != "" {
			out.AccountName = &st.AccountName
		}
		out.Scopes = st.Scopes
	}
	if st.LastError != "" {
		out.LastError = &st.LastError
	}
	return out, nil
}

func registerSpotify(mux *http.ServeMux, c *spotify.Client, baseURL string, proxies trust.Proxies, lp spotify.LoopbackPort) {
	origin := func(r *http.Request) string { return externalOrigin(r, baseURL, proxies) }
	status := func(w http.ResponseWriter, r *http.Request) {
		v, err := spotifyConnection(r, c, origin(r), lp)
		writeResult(w, r, v, err)
	}

	mux.HandleFunc("GET "+v1+"/spotify/connection", status)
	mux.HandleFunc("PUT "+v1+"/spotify/client-id", func(w http.ResponseWriter, r *http.Request) {
		var in SpotifyClientIDInput
		if err := decodeBody(r, &in); err != nil {
			writeResult[SpotifyConnection](w, r, nil, err)
			return
		}
		if err := c.SetClientID(r.Context(), in.ClientID); err != nil {
			writeResult[SpotifyConnection](w, r, nil, spotifyError(err))
			return
		}
		status(w, r)
	})
	mux.HandleFunc("POST "+v1+"/spotify/authorize", func(w http.ResponseWriter, r *http.Request) {
		var in SpotifyAuthorizeInput
		if err := decodeOptionalBody(r, &in); err != nil {
			writeResult[SpotifyAuthorization](w, r, nil, err)
			return
		}
		mode, uri := spotify.Redirect(origin(r), lp)
		req, err := c.BeginAuth(r.Context(), uri, in.ReturnTo)
		if err != nil {
			writeResult[SpotifyAuthorization](w, r, nil, spotifyError(err))
			return
		}
		writeJSON(w, r, http.StatusOK, &SpotifyAuthorization{AuthorizeURL: req.URL,
			Callback: SpotifyCallback{Mode: string(mode), RedirectURI: uri}, ExpiresAt: req.ExpiresAt.UnixMilli()})
	})
	mux.HandleFunc("POST "+v1+"/spotify/paste-back", func(w http.ResponseWriter, r *http.Request) {
		var in SpotifyPasteBackInput
		if err := decodeBody(r, &in); err != nil {
			writeResult[SpotifyConnection](w, r, nil, err)
			return
		}
		p, err := spotify.ParseCallbackURL(in.URL)
		if err == nil {
			_, err = c.Complete(r.Context(), p)
		}
		if err != nil {
			writeResult[SpotifyConnection](w, r, nil, spotifyError(err))
			return
		}
		status(w, r)
	})
	mux.HandleFunc("DELETE "+v1+"/spotify/connection", func(w http.ResponseWriter, r *http.Request) {
		if err := c.Disconnect(r.Context()); err != nil {
			writeResult[SpotifyConnection](w, r, nil, err)
			return
		}
		status(w, r)
	})

	// Direct callback (HTTPS or loopback HTTP). Not under /api/v1: Spotify redirects the
	// browser here. The single-use state, created by an authenticated
	// owner, authorizes the request; the browser then returns to the page
	// that started the flow with ?spotify=connected or ?spotify=error.
	mux.HandleFunc("GET "+spotify.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		returnTo, err := c.Complete(r.Context(), spotify.CallbackParams{State: q.Get("state"), Code: q.Get("code"), Error: q.Get("error")})
		if returnTo == "" {
			returnTo = "/"
		}
		result := url.Values{"spotify": {"connected"}}
		if err != nil {
			code := "internal"
			var he *httpError
			if errors.As(spotifyError(err), &he) {
				code = he.code
			} else {
				slog.Error("spotify callback", "err", err)
			}
			result = url.Values{"spotify": {"error"}, "spotify_error": {code}}
		}
		sep := "?"
		if strings.Contains(returnTo, "?") {
			sep = "&"
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, returnTo+sep+result.Encode(), http.StatusSeeOther)
	})
}

// decodeOptionalBody is decodeBody for requests whose body may be empty.
func decodeOptionalBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: invalid JSON body: %v", ErrBadRequest, err)
	}
	return nil
}
