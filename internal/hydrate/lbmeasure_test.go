//go:build measure

// Measures whether ListenBrainz's keyless batch endpoints could replace
// or speed up the MusicBrainz fallback stage (D27; proposal D36). Nothing
// here is wired into the worker. It makes no Spotify Web API calls; it
// calls labs.api.listenbrainz.org (acrr-lookup, acr-lookup,
// spotify-id-from-mbid), api.listenbrainz.org/1/metadata/recording,
// MusicBrainz (/ws/2/artist/{mbid}?inc=genres+url-rels, 1 request/s, as
// the reference for genres and Spotify artist links) and Spotify oEmbed
// (to check a sample of those links by name). Same sample as the D27
// measurement: top MEASURE_TOP tracks by plays plus MEASURE_TAIL random
// ones from the rest, on a throwaway data directory with an export
// imported. It prints aggregates; MEASURE_SPOTCHECK names a file that
// receives matches for a manual check (keep it out of the repo).
//
//	MEASURE_DATA_DIR=/tmp/x mise exec -- go test -tags measure -run MeasureListenBrainz -v -timeout 3h ./internal/hydrate/
package hydrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/metadata"
)

type lbClient struct {
	hc   *http.Client
	ua   string
	reqs map[string]int
	wait time.Duration // spent waiting on rate limits
}

var errLBStatus = errors.New("status")

// do sends a request, honouring X-RateLimit-* and retrying 429/503.
func (c *lbClient) do(ctx context.Context, kind, method, u string, body, out any) (int, error) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		req.Header.Set("User-Agent", c.ua)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		c.reqs[kind]++
		resp, err := c.hc.Do(req)
		if err != nil {
			if attempt < 3 {
				time.Sleep(2 * time.Second)
				continue
			}
			return 0, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resetIn, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Reset-In"))
		if (resp.StatusCode == 429 || resp.StatusCode == 503) && attempt < 6 {
			d := time.Duration(max(resetIn, 2)) * time.Second
			c.wait += d
			time.Sleep(d)
			continue
		}
		if rem := resp.Header.Get("X-RateLimit-Remaining"); rem == "0" && resetIn > 0 {
			d := time.Duration(resetIn) * time.Second
			c.wait += d
			time.Sleep(d)
		}
		if resp.StatusCode != 200 {
			return resp.StatusCode, fmt.Errorf("%w %d: %.200s", errLBStatus, resp.StatusCode, b)
		}
		if out != nil {
			if err := json.Unmarshal(b, out); err != nil {
				return resp.StatusCode, err
			}
		}
		return resp.StatusCode, nil
	}
}

const labsURL = "https://labs.api.listenbrainz.org"

type lbLookupHit struct {
	Index       int      `json:"index"`
	ACName      string   `json:"artist_credit_name"`
	RecName     string   `json:"recording_name"`
	RelName     string   `json:"release_name"`
	ArtistMBIDs []string `json:"artist_mbids"`
	RecMBID     string   `json:"recording_mbid"`
	RelMBID     string   `json:"release_mbid"`
}

type lbMeta struct {
	Artist struct {
		Name    string `json:"name"`
		Artists []struct {
			MBID string `json:"artist_mbid"`
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"artist"`
	Recording struct {
		Name             string   `json:"name"`
		Length           int64    `json:"length"`
		ISRCs            []string `json:"isrcs"`
		FirstReleaseDate string   `json:"first_release_date"`
	} `json:"recording"`
	Release struct {
		MBID  string `json:"mbid"`
		Name  string `json:"name"`
		CAAID int64  `json:"caa_id"`
	} `json:"release"`
	Tag struct {
		Artist []struct {
			ArtistMBID string `json:"artist_mbid"`
			Count      int    `json:"count"`
			GenreMBID  string `json:"genre_mbid"`
			Tag        string `json:"tag"`
		} `json:"artist"`
		Recording []struct {
			Count     int    `json:"count"`
			GenreMBID string `json:"genre_mbid"`
			Tag       string `json:"tag"`
		} `json:"recording"`
	} `json:"tag"`
}

type lbTrack struct {
	id, part string
	e        exportTrack
	stage    string
	hit      lbLookupHit
	meta     *lbMeta
	spIDs    []string
	verified bool // the export's Spotify track ID is among the recording's
	score    float64
	artist   string // best-matching credited artist MBID
	accepted bool
}

func TestMeasureListenBrainz(t *testing.T) {
	dir := os.Getenv("MEASURE_DATA_DIR")
	if dir == "" {
		t.Skip("MEASURE_DATA_DIR not set")
	}
	top, tail := lbEnvInt("MEASURE_TOP", 1000), lbEnvInt("MEASURE_TAIL", 1000)
	ctx := context.Background()
	d, err := db.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h := New(Options{DB: d})
	if _, err := h.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	// The D27 sample: top by priority, then PCG(1, 2)-shuffled rest.
	var all []string
	rs, err := d.Read.Query(`SELECT id FROM hydration_queue WHERE entity_type = 'track' ORDER BY priority DESC, id`)
	if err != nil {
		t.Fatal(err)
	}
	for rs.Next() {
		var s string
		rs.Scan(&s)
		all = append(all, s)
	}
	rs.Close()
	var queuedArtists int64
	d.Read.QueryRow(`SELECT COUNT(DISTINCT artist_name) FROM raw_export_rows WHERE uri IN
		(SELECT 'spotify:track:' || id FROM hydration_queue WHERE entity_type = 'track')`).Scan(&queuedArtists)
	topIDs := all[:min(top, len(all))]
	rest := append([]string(nil), all[len(topIDs):]...)
	r := rand.New(rand.NewPCG(1, 2))
	r.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	tailIDs := rest[:min(tail, len(rest))]
	var tracks []*lbTrack
	for _, set := range []struct {
		ids  []string
		part string
	}{{topIDs, "top"}, {tailIDs, "tail"}} {
		for _, id := range set.ids {
			e, ok, err := h.exportInfo(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				tracks = append(tracks, &lbTrack{id: id, part: set.part, e: e})
			}
		}
	}
	t.Logf("export: %d queued tracks, %d distinct export artist names; sample %d tracks", len(all), queuedArtists, len(tracks))

	c := &lbClient{hc: &http.Client{Timeout: 60 * time.Second}, ua: metadata.UserAgent("measure", ""), reqs: map[string]int{}}
	timing := map[string]time.Duration{}

	// 1. Name lookups, a cascade over the tracks still unmatched.
	type lookupStage struct {
		name, endpoint string
		item           func(e exportTrack) map[string]string
	}
	stages := []lookupStage{
		{"acrr", "acrr-lookup", func(e exportTrack) map[string]string {
			return map[string]string{"artist_credit_name": e.artist, "recording_name": e.track, "release_name": e.album}
		}},
		{"acrr-core", "acrr-lookup", func(e exportTrack) map[string]string {
			return map[string]string{"artist_credit_name": e.artist, "recording_name": metadata.StripQualifiers(e.track),
				"release_name": metadata.StripQualifiers(e.album)}
		}},
		{"acr", "acr-lookup", func(e exportTrack) map[string]string {
			return map[string]string{"artist_credit_name": e.artist, "recording_name": e.track}
		}},
		{"acr-core", "acr-lookup", func(e exportTrack) map[string]string {
			return map[string]string{"artist_credit_name": e.artist, "recording_name": metadata.StripQualifiers(e.track)}
		}},
	}
	const lookupBatch = 200
	t0 := time.Now()
	for _, st := range stages {
		var todo []*lbTrack
		for _, tr := range tracks {
			if tr.stage == "" {
				todo = append(todo, tr)
			}
		}
		s0 := time.Now()
		for i := 0; i < len(todo); i += lookupBatch {
			chunk := todo[i:min(i+lookupBatch, len(todo))]
			items := make([]map[string]string, len(chunk))
			for j, tr := range chunk {
				items[j] = st.item(tr.e)
			}
			var hits []lbLookupHit
			if _, err := c.do(ctx, st.endpoint, "POST", labsURL+"/"+st.endpoint+"/json", items, &hits); err != nil {
				t.Fatalf("%s: %v", st.name, err)
			}
			for _, hit := range hits {
				if hit.Index >= 0 && hit.Index < len(chunk) && hit.RecMBID != "" && chunk[hit.Index].stage == "" {
					chunk[hit.Index].stage, chunk[hit.Index].hit = st.name, hit
				}
			}
		}
		n := 0
		for _, tr := range todo {
			if tr.stage == st.name {
				n++
			}
		}
		t.Logf("lookup %-9s %4d of %4d remaining tracks found in %s", st.name, n, len(todo), time.Since(s0).Round(time.Millisecond))
	}
	timing["lookup"] = time.Since(t0)

	var hits []*lbTrack
	for _, tr := range tracks {
		if tr.stage != "" {
			hits = append(hits, tr)
		}
	}

	// 2. Batch size limits, on real MBIDs.
	mbids := make([]string, 0, len(hits))
	seen := map[string]bool{}
	for _, tr := range hits {
		if !seen[tr.hit.RecMBID] {
			seen[tr.hit.RecMBID] = true
			mbids = append(mbids, tr.hit.RecMBID)
		}
	}
	metaBatch := 0
	for _, n := range []int{25, 50, 100, 150, 200, 300, 400, 600, 1000} {
		if n > len(mbids) {
			break
		}
		var out map[string]json.RawMessage
		s0 := time.Now()
		code, err := c.do(ctx, "metadata", "GET", "https://api.listenbrainz.org/1/metadata/recording/?recording_mbids="+
			strings.Join(mbids[:n], ",")+"&inc="+url.QueryEscape("artist tag release"), nil, &out)
		t.Logf("batch probe metadata/recording n=%d: status %d, %d results, %s, err %v", n, code, len(out), time.Since(s0).Round(time.Millisecond), err)
		if err != nil {
			break
		}
		metaBatch = n
	}
	for _, n := range []int{500, 1000, 2000} {
		if n > len(mbids) {
			break
		}
		items := make([]map[string]string, n)
		for i := range items {
			items[i] = map[string]string{"recording_mbid": mbids[i]}
		}
		var out []json.RawMessage
		s0 := time.Now()
		code, err := c.do(ctx, "spotify-id-from-mbid", "POST", labsURL+"/spotify-id-from-mbid/json", items, &out)
		t.Logf("batch probe spotify-id-from-mbid n=%d: status %d, %d results, %s, err %v", n, code, len(out), time.Since(s0).Round(time.Millisecond), err)
		items2 := make([]map[string]string, n)
		for i := range items2 {
			tr := tracks[i%len(tracks)]
			items2[i] = map[string]string{"artist_credit_name": tr.e.artist, "recording_name": tr.e.track, "release_name": tr.e.album}
		}
		var out2 []json.RawMessage
		s0 = time.Now()
		code, err = c.do(ctx, "acrr-lookup", "POST", labsURL+"/acrr-lookup/json", items2, &out2)
		t.Logf("batch probe acrr-lookup n=%d: status %d, %d results, %s, err %v", n, code, len(out2), time.Since(s0).Round(time.Millisecond), err)
	}
	if metaBatch == 0 {
		t.Fatal("metadata/recording failed at every batch size")
	}
	metaBatch = min(metaBatch, 200)

	// 3. Recording metadata.
	t0 = time.Now()
	metas := map[string]*lbMeta{}
	for i := 0; i < len(mbids); i += metaBatch {
		chunk := mbids[i:min(i+metaBatch, len(mbids))]
		var out map[string]*lbMeta
		if _, err := c.do(ctx, "metadata", "GET", "https://api.listenbrainz.org/1/metadata/recording/?recording_mbids="+
			strings.Join(chunk, ",")+"&inc="+url.QueryEscape("artist tag release"), nil, &out); err != nil {
			t.Fatalf("metadata: %v", err)
		}
		for k, v := range out {
			metas[k] = v
		}
	}
	timing["metadata"] = time.Since(t0)

	// 4. Spotify track IDs ListenBrainz maps each recording to.
	t0 = time.Now()
	spIDs := map[string][]string{}
	for i := 0; i < len(mbids); i += lookupBatch {
		chunk := mbids[i:min(i+lookupBatch, len(mbids))]
		items := make([]map[string]string, len(chunk))
		for j, m := range chunk {
			items[j] = map[string]string{"recording_mbid": m}
		}
		var out []struct {
			RecMBID string   `json:"recording_mbid"`
			IDs     []string `json:"spotify_track_ids"`
		}
		if _, err := c.do(ctx, "spotify-id-from-mbid", "POST", labsURL+"/spotify-id-from-mbid/json", items, &out); err != nil {
			t.Fatalf("spotify-id-from-mbid: %v", err)
		}
		for _, o := range out {
			spIDs[o.RecMBID] = append(spIDs[o.RecMBID], o.IDs...)
		}
	}
	timing["spotify-ids"] = time.Since(t0)

	// 5. Score: the D27 track matcher on the recording's title, best
	// credited artist, release and length; a verified Spotify ID wins.
	for _, tr := range hits {
		tr.meta = metas[tr.hit.RecMBID]
		tr.spIDs = spIDs[tr.hit.RecMBID]
		for _, s := range tr.spIDs {
			if s == tr.id {
				tr.verified = true
			}
		}
		recName, relName, acName := tr.hit.RecName, tr.hit.RelName, tr.hit.ACName
		var length int64
		type cand struct{ mbid, name string }
		var cands []cand
		if tr.meta != nil {
			recName, length = tr.meta.Recording.Name, tr.meta.Recording.Length
			for _, a := range tr.meta.Artist.Artists {
				cands = append(cands, cand{a.MBID, a.Name})
			}
		} else {
			for _, m := range tr.hit.ArtistMBIDs {
				cands = append(cands, cand{m, acName})
			}
		}
		bestName, bestSim := acName, metadata.NameSim(tr.e.artist, acName)
		for _, cd := range cands {
			if s := metadata.NameSim(tr.e.artist, cd.name); s >= 0.9 && (tr.artist == "" || s > metadata.NameSim(tr.e.artist, bestName)) {
				tr.artist, bestName, bestSim = cd.mbid, cd.name, s
			}
		}
		_ = bestSim
		dt := metadata.DeezerTrack{Title: recName, Duration: int((length + 500) / 1000)}
		dt.Artist.Name, dt.Album.Title = bestName, relName
		tr.score = metadata.TrackScore(metadata.TrackQuery{Title: tr.e.track, Artist: tr.e.artist, Album: tr.e.album,
			Duration: tr.e.dur}, dt)
		tr.accepted = tr.artist != "" && (tr.verified || tr.score >= metadata.MinTrackScore)
	}

	// Track aggregates.
	for _, part := range []string{"top", "tail"} {
		var n, found, ver, acc, accUnver, withLen, withISRC, withCAA, withRecGenre int
		byStage := map[string]int{}
		for _, tr := range tracks {
			if tr.part != part {
				continue
			}
			n++
			if tr.stage == "" {
				continue
			}
			found++
			if tr.verified {
				ver++
			}
			if !tr.accepted {
				continue
			}
			acc++
			byStage[tr.stage]++
			if !tr.verified {
				accUnver++
			}
			if tr.meta != nil {
				if tr.meta.Recording.Length > 0 {
					withLen++
				}
				if len(tr.meta.Recording.ISRCs) > 0 {
					withISRC++
				}
				if tr.meta.Release.CAAID != 0 {
					withCAA++
				}
				for _, tg := range tr.meta.Tag.Recording {
					if tg.GenreMBID != "" {
						withRecGenre++
						break
					}
				}
			}
		}
		pct := func(a int) string { return fmt.Sprintf("%d (%.1f%%)", a, 100*float64(a)/float64(max(n, 1))) }
		t.Logf("%-4s %d tracks: lookup hit %s, Spotify-ID verified %s, accepted %s (unverified %d) by stage %v",
			part, n, pct(found), pct(ver), pct(acc), accUnver, byStage)
		t.Logf("%-4s accepted with length %s, ISRC %s, cover art (caa_id) %s, recording genre tag %s",
			part, pct(withLen), pct(withISRC), pct(withCAA), pct(withRecGenre))
	}
	hist := map[string]int{}
	for _, tr := range hits {
		k := fmt.Sprintf("%.2f", float64(int(tr.score*20))/20)
		if tr.verified {
			k = "verified " + k
		}
		if tr.artist == "" {
			k = "no-artist " + k
		}
		hist[k]++
	}
	t.Logf("hit score histogram (bucket:count): %v", hist)

	// 6. Artists: one MBID per export artist name, by vote over its
	// accepted tracks (verified tracks count double).
	type artistInfo struct {
		name               string
		votes              map[string]int
		mbid               string
		verified           bool
		lbGenres, mbGenres []string
		tracks             int
	}
	artists := map[string]*artistInfo{}
	tagsByArtist := map[string]map[string]int{} // mbid -> genre tag -> count
	allTagsByArtist := map[string]map[string]int{}
	for _, tr := range tracks {
		key := ProvArtistID(tr.e.artist)
		a := artists[key]
		if a == nil {
			a = &artistInfo{name: tr.e.artist, votes: map[string]int{}}
			artists[key] = a
		}
		a.tracks++
		if !tr.accepted {
			continue
		}
		w := 1
		if tr.verified {
			w, a.verified = 2, true
		}
		a.votes[tr.artist] += w
		if tr.meta != nil {
			for _, tg := range tr.meta.Tag.Artist {
				if allTagsByArtist[tg.ArtistMBID] == nil {
					allTagsByArtist[tg.ArtistMBID], tagsByArtist[tg.ArtistMBID] = map[string]int{}, map[string]int{}
				}
				allTagsByArtist[tg.ArtistMBID][tg.Tag] = tg.Count
				if tg.GenreMBID != "" {
					tagsByArtist[tg.ArtistMBID][tg.Tag] = tg.Count
				}
			}
		}
	}
	var withMBID, withLBGenres, multiVote int
	for _, a := range artists {
		for m, v := range a.votes {
			if a.mbid == "" || v > a.votes[a.mbid] || (v == a.votes[a.mbid] && m < a.mbid) {
				a.mbid = m
			}
		}
		if len(a.votes) > 1 {
			multiVote++
		}
		if a.mbid == "" {
			continue
		}
		withMBID++
		var gs []metadata.MBGenre
		for g, n := range tagsByArtist[a.mbid] {
			gs = append(gs, metadata.MBGenre{Name: g, Count: n})
		}
		a.lbGenres = metadata.TopGenres(gs, maxGenres)
		if len(a.lbGenres) > 0 {
			withLBGenres++
		}
	}
	// Track coverage of artist MBIDs, as D27 reported for MusicBrainz.
	for _, part := range []string{"top", "tail"} {
		var n, cov int
		for _, tr := range tracks {
			if tr.part != part {
				continue
			}
			n++
			if artists[ProvArtistID(tr.e.artist)].mbid != "" {
				cov++
			}
		}
		t.Logf("%-4s artist MBID covers %d of %d tracks (%.1f%%)", part, cov, n, 100*float64(cov)/float64(max(n, 1)))
	}
	t.Logf("artists: %d export names; %d with an MBID (%d with conflicting votes), %d with ListenBrainz genres",
		len(artists), withMBID, multiVote, withLBGenres)

	// 7. MusicBrainz reference: curated genres and Spotify links, one
	// request per artist at 1/s.
	mb := &lbClient{hc: c.hc, ua: c.ua, reqs: map[string]int{}}
	t0 = time.Now()
	var keys []string
	for k, a := range artists {
		if a.mbid != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var mbGenresN, bothGenres, sameList, sameSet, lbOnly, mbOnly, spotify1, spotifyN int
	var jac float64
	type spLink struct{ name, id string }
	var links []spLink
	last := time.Time{}
	for _, k := range keys {
		a := artists[k]
		if wait := time.Second - time.Since(last); wait > 0 {
			time.Sleep(wait)
		}
		last = time.Now()
		var ar struct {
			Genres    []metadata.MBGenre `json:"genres"`
			Relations []struct {
				URL struct {
					Resource string `json:"resource"`
				} `json:"url"`
			} `json:"relations"`
		}
		if _, err := mb.do(ctx, "musicbrainz", "GET", "https://musicbrainz.org/ws/2/artist/"+a.mbid+"?inc=genres+url-rels&fmt=json", nil, &ar); err != nil {
			t.Logf("musicbrainz %s: %v", a.mbid, err)
			continue
		}
		mg := metadata.TopGenres(ar.Genres, maxGenres)
		a.mbGenres = mg
		if len(mg) > 0 {
			mbGenresN++
		}
		switch {
		case len(mg) > 0 && len(a.lbGenres) > 0:
			bothGenres++
			if strings.Join(mg, ",") == strings.Join(a.lbGenres, ",") {
				sameList++
			}
			ms, ls := map[string]bool{}, map[string]bool{}
			for _, g := range mg {
				ms[g] = true
			}
			inter := 0
			for _, g := range a.lbGenres {
				ls[g] = true
				if ms[g] {
					inter++
				}
			}
			if inter == len(ms) && inter == len(ls) {
				sameSet++
			}
			jac += float64(inter) / float64(len(ms)+len(ls)-inter)
		case len(a.lbGenres) > 0:
			lbOnly++
		case len(mg) > 0:
			mbOnly++
		}
		var sp []string
		for _, rel := range ar.Relations {
			if u := rel.URL.Resource; strings.Contains(u, "open.spotify.com/artist/") {
				id := u[strings.LastIndex(u, "/")+1:]
				if i := strings.IndexAny(id, "?#"); i >= 0 {
					id = id[:i]
				}
				sp = append(sp, id)
			}
		}
		switch {
		case len(sp) == 1:
			spotify1++
			links = append(links, spLink{a.name, sp[0]})
		case len(sp) > 1:
			spotifyN++
		}
	}
	timing["musicbrainz"] = time.Since(t0)
	t.Logf("MusicBrainz reference over %d artists in %s: %d with curated genres; ListenBrainz genres on the same artists: both %d (same top list %d, same set %d, mean Jaccard %.2f), ListenBrainz only %d, MusicBrainz only %d",
		len(keys), timing["musicbrainz"].Round(time.Second), mbGenresN, bothGenres, sameList, sameSet, jac/float64(max(bothGenres, 1)), lbOnly, mbOnly)
	t.Logf("Spotify artist links (url-rels): %d artists with exactly one, %d with several, %d with none (of %d with an MBID)",
		spotify1, spotifyN, len(keys)-spotify1-spotifyN, len(keys))
	// Track coverage through those artists.
	spByArtist := map[string]bool{}
	for _, l := range links {
		spByArtist[ProvArtistID(l.name)] = true
	}
	for _, part := range []string{"top", "tail"} {
		var n, cov int
		for _, tr := range tracks {
			if tr.part == part {
				n++
				if spByArtist[ProvArtistID(tr.e.artist)] {
					cov++
				}
			}
		}
		t.Logf("%-4s a Spotify artist ID from url-rels covers %d of %d tracks (%.1f%%)", part, cov, n, 100*float64(cov)/float64(max(n, 1)))
	}

	// 8. Check 100 random Spotify links by oEmbed name.
	oe := metadata.NewOEmbed("", c.ua, nil, nil)
	r.Shuffle(len(links), func(i, j int) { links[i], links[j] = links[j], links[i] })
	var okName, badName, gone int
	var bad []string
	for _, l := range links[:min(100, len(links))] {
		res, err := oe.Lookup(ctx, "spotify:artist:"+l.id)
		switch {
		case errors.Is(err, metadata.ErrNotFound):
			gone++
		case err != nil:
			t.Logf("oembed %s: %v", l.id, err)
		case metadata.NameSim(l.name, res.Title) >= 0.9:
			okName++
		default:
			badName++
			bad = append(bad, fmt.Sprintf("%q vs %q", l.name, res.Title))
		}
	}
	t.Logf("oEmbed check of %d url-rel Spotify artist IDs: %d same name, %d different name %v, %d not found",
		min(100, len(links)), okName, badName, bad, gone)

	t.Logf("requests: ListenBrainz %v, MusicBrainz %v; rate-limit waits %s", c.reqs, mb.reqs, c.wait.Round(time.Millisecond))
	t.Logf("timing: %v", timing)

	if f := os.Getenv("MEASURE_SPOTCHECK"); f != "" {
		var unver, ver []*lbTrack
		for _, tr := range hits {
			if tr.accepted && !tr.verified {
				unver = append(unver, tr)
			} else if tr.accepted {
				ver = append(ver, tr)
			}
		}
		r.Shuffle(len(unver), func(i, j int) { unver[i], unver[j] = unver[j], unver[i] })
		r.Shuffle(len(ver), func(i, j int) { ver[i], ver[j] = ver[j], ver[i] })
		var b strings.Builder
		line := func(tr *lbTrack) {
			var recName, ac string
			var length int64
			if tr.meta != nil {
				recName, ac, length = tr.meta.Recording.Name, tr.meta.Artist.Name, tr.meta.Recording.Length
			}
			fmt.Fprintf(&b, "%s\t%.2f\t%s | %s | %s (exp %d, min %d)\t=> %s | %s | %s (len %d)\trec=%s spids=%v\n",
				tr.stage, tr.score, tr.e.track, tr.e.artist, tr.e.album, tr.e.dur.ExpectedMs, tr.e.dur.MinMs,
				recName, ac, tr.hit.RelName, length, tr.hit.RecMBID, tr.spIDs)
		}
		b.WriteString("# accepted, not verified by Spotify ID\n")
		for _, tr := range unver[:min(30, len(unver))] {
			line(tr)
		}
		b.WriteString("# accepted, verified\n")
		for _, tr := range ver[:min(10, len(ver))] {
			line(tr)
		}
		b.WriteString("# rejected hits\n")
		var rej []*lbTrack
		for _, tr := range hits {
			if !tr.accepted {
				rej = append(rej, tr)
			}
		}
		r.Shuffle(len(rej), func(i, j int) { rej[i], rej[j] = rej[j], rej[i] })
		for _, tr := range rej[:min(20, len(rej))] {
			line(tr)
		}
		b.WriteString("# genres, 20 artists with both (ListenBrainz)\n")
		n := 0
		for _, k := range keys {
			if a := artists[k]; len(a.lbGenres) > 0 && n < 20 {
				fmt.Fprintf(&b, "%s\tLB %v\tMB %v\tall tags %d\n", a.name, a.lbGenres, a.mbGenres, len(allTagsByArtist[a.mbid]))
				n++
			}
		}
		os.WriteFile(f, []byte(b.String()), 0o600)
	}
}

func lbEnvInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return def
}
