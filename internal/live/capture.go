package live

import (
	"context"
	"strings"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/capture"
	"spotussi/internal/spotify"
)

// Capture is what the live backend reads from the capture service.
type Capture interface {
	Status() capture.Status
	NowPlaying() (st *spotify.PlayerState, observedAt, startedAt int64)
}

// NowPlaying reports the poller's last snapshot.
func (b *Backend) NowPlaying(ctx context.Context) (*api.NowPlaying, error) {
	out := &api.NowPlaying{}
	if b.capture == nil {
		return out, nil
	}
	st, observed, started := b.capture.NowPlaying()
	if st == nil {
		return out, nil
	}
	out.ObservedAt = &observed
	if st.Item == nil || st.CurrentlyPlayingType == "episode" || st.CurrentlyPlayingType == "ad" {
		return out, nil // not music: nothing to show
	}
	tr := trackRef(st.Item)
	out.Track = &tr
	out.IsPlaying = st.IsPlaying
	out.ProgressMs = st.ProgressMs
	if started > 0 {
		out.StartedAt = &started
	}
	if st.Device != nil && st.Device.Name != "" {
		out.Device = &api.Device{Name: st.Device.Name, Type: st.Device.Type}
	}
	if st.Context != nil {
		out.Context = contextRef(st.Context.URI)
	}
	shuffle := st.ShuffleState
	out.Shuffle = &shuffle
	return out, nil
}

// trackRef builds a TrackRef from the track object Spotify returned.
func trackRef(t *spotify.Track) api.TrackRef {
	r := api.TrackRef{ID: t.Key(), Name: t.Name, Artists: []api.ArtistLite{}}
	if r.Name == "" {
		r.Name = t.URI
	}
	if t.DurationMs > 0 {
		d := t.DurationMs
		r.DurationMs = &d
	}
	for _, a := range t.Artists {
		id := a.ID
		if id == "" {
			id = a.URI
		}
		r.Artists = append(r.Artists, api.ArtistLite{ID: id, Name: a.Name})
	}
	if t.Album.ID != "" || t.Album.Name != "" {
		r.Album = &api.AlbumLite{ID: t.Album.ID, Name: t.Album.Name}
	}
	if len(t.Album.Images) > 0 && t.Album.Images[0].URL != "" {
		u := t.Album.Images[0].URL
		r.ImageURL = &u
	}
	return r
}

// spotifyOwnedPrefix marks Spotify-made playlists (Daily Mix, Discover
// Weekly, ...), which apps cannot read (D17).
const spotifyOwnedPrefix = "37i9dQZ"

// contextRef parses a context URI (spotify:playlist:ID,
// spotify:user:NAME:collection, ...). Names are resolved by the catalog
// in a later milestone.
func contextRef(uri string) *api.ContextRef {
	if uri == "" {
		return nil
	}
	parts := strings.Split(uri, ":")
	c := &api.ContextRef{URI: uri, Type: "other", ID: parts[len(parts)-1], NameSource: "unresolved"}
	switch {
	case len(parts) == 3 && (parts[1] == "playlist" || parts[1] == "album" || parts[1] == "artist"):
		c.Type = parts[1]
	case parts[len(parts)-1] == "collection":
		c.Type = "collection"
	}
	c.SpotifyOwned = c.Type == "playlist" && strings.HasPrefix(c.ID, spotifyOwnedPrefix)
	return c
}

func msPtr(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	v := t.UnixMilli()
	return &v
}

// captureStatus converts the capture service's status.
func (b *Backend) captureStatus() api.CaptureStatus {
	out := api.CaptureStatus{State: capture.StateStopped}
	if b.capture != nil {
		st := b.capture.Status()
		out.State = st.State
		out.LastPollAt, out.LastPlayAt = msPtr(st.LastPollAt), msPtr(st.LastPlayAt)
		if st.PollInterval > 0 {
			iv := st.PollInterval.Milliseconds()
			out.PollIntervalMs = &iv
		}
		out.NextPollAt = msPtr(st.NextPollAt)
		out.LastError, out.LastErrorAt = strPtr(st.LastError), msPtr(st.LastErrorAt)
		out.Backstop = &api.CaptureBackstop{LastRunAt: msPtr(st.RecentLastRunAt), LastError: strPtr(st.RecentLastError),
			ItemsStored: st.RecentInserted}
		if c := st.Calibration; c != nil {
			out.Calibration = &api.CaptureCalibration{Samples: c.Samples, Discriminating: c.Discriminating,
				Verdict: c.Verdict, StartOffset: offsetSummary(c.Start), EndOffset: offsetSummary(c.End),
				UpdatedAt: msPtr(c.UpdatedAt)}
		}
	}
	if b.limiter != nil {
		ls := b.limiter()
		req := map[string]int64{}
		for k, v := range ls.Requests {
			req[k] = v
		}
		qb := map[string]int64{}
		for k, v := range ls.QuotaBlocked {
			qb[k] = v.UnixMilli()
		}
		out.RateLimit = &api.RateLimitStatus{QuotaBlocked: qb, RatePerSec: ls.RatePerSec, EffectiveRatePerSec: ls.EffectiveRatePerSec,
			Burst: ls.Burst, LiveReserve: ls.LiveReserve, BlockedUntil: msPtr(ls.BlockedUntil), Throttled: ls.Throttled,
			QuotaExceeded: ls.QuotaExceeded, Last429At: msPtr(ls.Last429At), Requests: req}
	}
	return out
}

func offsetSummary(o capture.OffsetStats) api.OffsetSummary {
	return api.OffsetSummary{MedianMs: o.MedianMs, P10Ms: o.P10Ms, P90Ms: o.P90Ms, HitRate: o.HitRate}
}
