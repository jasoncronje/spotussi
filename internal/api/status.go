package api

import (
	"net/http"

	"spotussi/internal/spotify"
)

// systemStatus serves /system/status: the backend's view (or an empty
// live-mode status until the capture and query layers exist), with the
// auth mode and, outside mock mode, the real Spotify connection overlaid.
// Mock mode keeps its synthetic "connected" state so dashboards render;
// its Spotify endpoints still work against the mock's scratch database.
func systemStatus(o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := &SystemStatus{
			Spotify: SpotifyStatus{State: string(spotify.StateNotConfigured)},
			Capture: CaptureStatus{State: "stopped"},
			Imports: []ImportJob{},
			Catalog: CatalogStatus{State: "disabled", Fallbacks: []CatalogFallback{}},
			Derive:  DeriveStatus{State: "idle"},
			Library: DisabledLibraryStatus(),
		}
		if o.Backend != nil {
			var err error
			if st, err = o.Backend.SystemStatus(r.Context()); err != nil {
				writeResult[SystemStatus](w, r, nil, err)
				return
			}
		}
		st.Auth = AuthStatus{Mode: authModeName(o.Auth)}
		if o.Spotify != nil && o.Mode != "mock" {
			s, err := o.Spotify.Status(r.Context())
			if err != nil {
				writeResult[SystemStatus](w, r, nil, err)
				return
			}
			st.Spotify = SpotifyStatus{State: string(s.State), Scopes: s.Scopes}
			if s.State != spotify.StateNotConfigured {
				name := s.AccountName
				if name == "" {
					name = s.AccountID
				}
				st.Spotify.AccountName, st.Spotify.ConnectedAt = &name, &s.ConnectedAt
			}
		}
		if st.Spotify.Scopes == nil {
			st.Spotify.Scopes = []string{}
		}
		writeJSON(w, r, http.StatusOK, st)
	}
}

// DisabledLibraryStatus is the library status of an instance without
// library sync.
func DisabledLibraryStatus() LibraryStatus {
	pending := LibraryListStatus{State: "pending"}
	return LibraryStatus{State: "disabled", Lists: LibraryLists{Tracks: pending, Albums: pending, Playlists: pending}}
}
