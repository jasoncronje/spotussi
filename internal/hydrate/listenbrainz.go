package hydrate

import (
	"context"
	"database/sql"

	"spotussi/internal/metadata"
)

// ---------- ListenBrainz (D37) ----------
//
// A first pass before MusicBrainz that finds a track's artist and its
// MusicBrainz genres through ListenBrainz's keyless batch endpoints, a
// batch of up to 100 tracks at a time: name lookups (with the album, then
// with qualifiers stripped, then without the album), the Spotify track
// IDs ListenBrainz links each found recording to, and the recordings'
// metadata (length, credited artists and their tags). A hit is accepted
// when the D27 track matcher accepts it or ListenBrainz links it to the
// export's own Spotify track ID (verified). The artist MBID is the
// credited artist named like the export artist, by vote over the batch's
// accepted tracks (verified ones count double). Genres are its tags that
// are MusicBrainz genres, mapped as D27 maps MusicBrainz's.
//
// It writes only what the MusicBrainz stage writes (the MBID and genres
// of the provisional artist), and that stage skips the artists it
// matched. A track is matched when its artist is; an artist stays
// unmatched (MusicBrainz tries it) until one of its tracks matches.

// lbMinArtistSim is how closely a credited artist's name must match the
// export artist to take its MBID.
const lbMinArtistSim = 0.9

// lbTrack is a batch entry with what ListenBrainz found for it.
type lbTrack struct {
	it       fbItem
	e        exportTrack
	ok       bool // has export names
	artistID string
	hit      *metadata.LBHit
	mbid     string  // the artist MBID the track votes for
	conf     float64 // 0: not accepted
	verified bool
}

func (h *Hydrator) fbListenBrainz(ctx context.Context, items []fbItem) (int, error) {
	lb := h.o.Fallbacks.ListenBrainz
	ts := make([]*lbTrack, len(items))
	matched := map[string]fbResult{} // provisional artist ID -> its settled match
	var todo []*lbTrack
	for i, it := range items {
		t := &lbTrack{it: it}
		ts[i] = t
		e, ok, err := h.exportInfo(ctx, it.id)
		if err != nil {
			return 0, err
		}
		t.e, t.ok = e, ok
		if !ok {
			continue
		}
		t.artistID = ProvArtistID(e.artist)
		if _, seen := matched[t.artistID]; !seen {
			status, ext, conf, done, err := h.fbFinal(ctx, metadata.SourceListenBrainz, "artist", t.artistID)
			if err != nil {
				return 0, err
			}
			if done && status == "matched" {
				matched[t.artistID] = fbResult{matched: true, externalID: ext, conf: conf}
			}
		}
		if _, known := matched[t.artistID]; !known {
			todo = append(todo, t) // artist already found: nothing to look up
		}
	}

	var recs map[string]metadata.LBRecording
	var findErr error
	if len(todo) > 0 {
		recs, findErr = h.lbFind(ctx, lb, todo)
		if findErr != nil {
			if ctx.Err() != nil {
				return 0, nil
			}
			// Retried later, and left to MusicBrainz after 3 failures.
			if err := h.fbFailed(ctx, metadata.SourceListenBrainz, findErr, entries(todo)...); err != nil {
				return 0, err
			}
			todo = nil
		}
	}

	// One MBID per new artist, by vote.
	type tally struct {
		votes    map[string]int
		conf     map[string]float64 // best confidence per MBID
		genres   map[string][]metadata.MBGenre
		name     string
		verified map[string]bool
	}
	tallies := map[string]*tally{}
	var order []string
	for _, t := range todo {
		if t.conf == 0 {
			continue
		}
		tl := tallies[t.artistID]
		if tl == nil {
			tl = &tally{votes: map[string]int{}, conf: map[string]float64{}, genres: map[string][]metadata.MBGenre{},
				name: t.e.artist, verified: map[string]bool{}}
			tallies[t.artistID] = tl
			order = append(order, t.artistID)
		}
		w := 1
		if t.verified {
			w = 2
			tl.verified[t.mbid] = true
		}
		tl.votes[t.mbid] += w
		tl.conf[t.mbid] = max(tl.conf[t.mbid], t.conf)
	}
	now := h.now().UnixMilli()
	n := 0
	err := h.tx(ctx, func(tx *sql.Tx) error {
		for _, artistID := range order {
			tl := tallies[artistID]
			mbid := ""
			for m, v := range tl.votes {
				if mbid == "" || v > tl.votes[mbid] || (v == tl.votes[mbid] && m < mbid) {
					mbid = m
				}
			}
			conf := tl.conf[mbid]
			if conf < minNameConf {
				continue
			}
			var gs []metadata.MBGenre
			for _, t := range todo {
				if t.artistID == artistID && t.mbid == mbid && t.hit != nil {
					if r, ok := recs[t.hit.RecordingID]; ok {
						if gs = r.ArtistGenres(mbid); len(gs) > 0 {
							break
						}
					}
				}
			}
			if err := h.lbWriteArtist(ctx, tx, artistID, tl.name, mbid, conf, metadata.TopGenres(gs, maxGenres), now); err != nil {
				return err
			}
			matched[artistID] = fbResult{matched: true, externalID: mbid, conf: conf}
		}
		for _, t := range ts {
			if findErr != nil && t.ok {
				if _, known := matched[t.artistID]; !known {
					continue // failed above
				}
			}
			n++
			status, res := "no_match", matched[t.artistID]
			if t.ok && res.matched {
				status = "matched"
			}
			if err := setFBState(ctx, tx, metadata.SourceListenBrainz, t.it.typ, t.it.id, status, res.externalID, res.conf,
				t.it.attempts, 0, "", now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for range n {
		h.fbDone(metadata.SourceListenBrainz)
	}
	if len(order) > 0 {
		h.changed(len(order))
	}
	if n == 0 && findErr != nil {
		return 0, findErr // the stage backs off
	}
	return n, nil
}

func entries(ts []*lbTrack) []fbItem {
	out := make([]fbItem, len(ts))
	for i, t := range ts {
		out[i] = t.it
	}
	return out
}

// lbFind looks the tracks up and scores what it finds. It returns the
// found recordings' metadata (for the genres).
func (h *Hydrator) lbFind(ctx context.Context, lb *metadata.ListenBrainz, ts []*lbTrack) (map[string]metadata.LBRecording, error) {
	type pass struct {
		withRelease, stripped bool
	}
	for _, p := range []pass{{true, false}, {true, true}, {false, false}, {false, true}} {
		var batch []*lbTrack
		var items []metadata.LBLookup
		for _, t := range ts {
			if t.hit != nil {
				continue
			}
			q := metadata.LBLookup{Artist: t.e.artist, Recording: t.e.track, Release: t.e.album}
			if p.stripped {
				q.Recording, q.Release = metadata.StripQualifiers(q.Recording), metadata.StripQualifiers(q.Release)
				if q.Recording == t.e.track && (!p.withRelease || q.Release == t.e.album) {
					continue // the same lookup as the unstripped pass
				}
			}
			if p.withRelease && q.Release == "" {
				continue
			}
			batch, items = append(batch, t), append(items, q)
		}
		if len(items) == 0 {
			continue
		}
		hits, err := lb.Lookup(ctx, items, p.withRelease)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			if t := batch[hit.Index]; t.hit == nil {
				t.hit = &hit
			}
		}
	}
	var mbids []string
	seen := map[string]bool{}
	for _, t := range ts {
		if t.hit != nil && !seen[t.hit.RecordingID] {
			seen[t.hit.RecordingID] = true
			mbids = append(mbids, t.hit.RecordingID)
		}
	}
	if len(mbids) == 0 {
		return nil, nil
	}
	recs, err := lb.Recordings(ctx, mbids)
	if err != nil {
		return nil, err
	}
	spIDs, err := lb.SpotifyTrackIDs(ctx, mbids)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if t.hit != nil {
			lbScore(t, recs[t.hit.RecordingID], spIDs[t.hit.RecordingID])
		}
	}
	return recs, nil
}

// lbScore accepts or rejects a track's hit and picks its artist MBID.
func lbScore(t *lbTrack, rec metadata.LBRecording, spIDs []string) {
	for _, id := range spIDs {
		if id == t.it.id {
			t.verified = true
		}
	}
	title, length := t.hit.Recording, int64(0)
	artists := rec.Artist.Artists
	if rec.Recording.Name != "" {
		title, length = rec.Recording.Name, rec.Recording.LengthMs
	}
	if len(artists) == 0 && len(t.hit.ArtistMBIDs) == 1 {
		artists = []metadata.LBArtist{{ID: t.hit.ArtistMBIDs[0], Name: t.hit.ArtistName}}
	}
	best, bestSim := metadata.LBArtist{}, 0.0
	for _, a := range artists {
		if s := metadata.NameSim(t.e.artist, a.Name); s >= lbMinArtistSim && s > bestSim {
			best, bestSim = a, s
		}
	}
	if best.ID == "" {
		return
	}
	dt := metadata.DeezerTrack{Title: title, Duration: int((length + 500) / 1000)}
	dt.Artist.Name, dt.Album.Title = best.Name, t.hit.Release
	score := metadata.TrackScore(metadata.TrackQuery{Title: t.e.track, Artist: t.e.artist, Album: t.e.album,
		Duration: t.e.dur}, dt)
	switch {
	case t.verified:
		t.mbid, t.conf = best.ID, 0.95
	case score >= metadata.MinTrackScore:
		t.mbid, t.conf = best.ID, round2(0.95*score)
	}
}

// lbWriteArtist records a provisional artist's MBID and genres. The MBID
// replaces another source's only when its genres do (a higher
// confidence), so the two stay together.
func (h *Hydrator) lbWriteArtist(ctx context.Context, tx *sql.Tx, artistID, name, mbid string, conf float64,
	genres []string, now int64) error {
	if err := fbArtistRow(ctx, tx, artistID, name, now); err != nil {
		return err
	}
	own := false
	if len(genres) > 0 {
		ok, err := claim(ctx, tx, "artist", artistID, "genres", metadata.SourceListenBrainz, conf, now)
		if err != nil {
			return err
		}
		if ok {
			own = true
			if _, err := tx.ExecContext(ctx, `DELETE FROM artist_genres WHERE artist_id = ?`, artistID); err != nil {
				return err
			}
			for _, g := range genres {
				if _, err := tx.ExecContext(ctx, `INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?)
					ON CONFLICT DO NOTHING`, artistID, g); err != nil {
					return err
				}
			}
		}
	}
	q := `UPDATE fallback_artists SET mbid = COALESCE(mbid, ?) WHERE id = ?`
	if own {
		q = `UPDATE fallback_artists SET mbid = ? WHERE id = ?`
	}
	if _, err := tx.ExecContext(ctx, q, mbid, artistID); err != nil {
		return err
	}
	return setFBState(ctx, tx, metadata.SourceListenBrainz, "artist", artistID, "matched", mbid, conf, 0, 0, "", now)
}
