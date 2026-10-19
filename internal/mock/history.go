package mock

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"spotussi/internal/api"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
)

// ---------- history ----------

func (b *Backend) History(ctx context.Context, p api.HistoryParams) (*api.History, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, err := b.resolve(p.Range)
	if err != nil {
		return nil, err
	}
	scope, err := b.scope(p.Scope)
	if err != nil {
		return nil, err
	}
	before := int32(len(b.ds.Plays)) // cursor: only plays with a lower index
	if p.Cursor != "" {
		n, err := strconv.ParseInt(p.Cursor, 36, 32)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%w: invalid cursor", api.ErrBadRequest)
		}
		before = int32(n)
	}
	match := func(i int32) bool {
		pl := b.ds.Plays[i]
		if !scope(pl) {
			return false
		}
		if p.Skipped != nil && pl.Has(refdata.FlagSkipped) != *p.Skipped {
			return false
		}
		if p.Shuffle != nil && (!pl.Has(refdata.FlagShuffleKnown) || pl.Has(refdata.FlagShuffle) != *p.Shuffle) {
			return false
		}
		if p.Counted != nil && b.counted[i] != *p.Counted {
			return false
		}
		if p.Device != "" && (pl.Device < 0 || b.ds.Devices[pl.Device].Name != p.Device) {
			return false
		}
		if p.Platform != "" && (pl.Platform < 0 || b.ds.Platforms[pl.Platform] != p.Platform) {
			return false
		}
		return true
	}
	plays := b.inRange(cur)
	out := &api.History{Range: wireRange(cur), Items: []api.Play{}}
	more := false
	for k := len(plays) - 1; k >= 0; k-- {
		i := plays[k]
		if !match(i) {
			continue
		}
		out.Total++
		if i >= before {
			continue
		}
		if len(out.Items) < p.Limit {
			out.Items = append(out.Items, b.wirePlay(i))
		} else {
			more = true
		}
	}
	if more {
		last, _ := b.parsePlayID(out.Items[len(out.Items)-1].ID)
		out.NextCursor = ptr(strconv.FormatInt(int64(last), 36))
	}
	return out, nil
}

func (b *Backend) Devices(ctx context.Context, spec query.Spec) (*api.Devices, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, err := b.resolve(spec)
	if err != nil {
		return nil, err
	}
	dev := make([]measure, len(b.ds.Devices))
	plat := make([]measure, len(b.ds.Platforms))
	for _, i := range b.inRange(cur) {
		p := b.ds.Plays[i]
		if p.Device >= 0 {
			b.add(&dev[p.Device], i)
		}
		if p.Platform >= 0 {
			b.add(&plat[p.Platform], i)
		}
	}
	out := &api.Devices{Range: wireRange(cur), Devices: []api.DeviceUsage{}, Platforms: []api.PlatformUsage{}}
	for d, m := range dev {
		if m.ms > 0 || m.plays > 0 {
			x := b.ds.Devices[d]
			out.Devices = append(out.Devices, api.DeviceUsage{Name: x.Name, Type: x.Type, Plays: m.plays, MsPlayed: m.ms})
		}
	}
	for pl, m := range plat {
		if m.ms > 0 || m.plays > 0 {
			out.Platforms = append(out.Platforms, api.PlatformUsage{Name: b.ds.Platforms[pl], Plays: m.plays, MsPlayed: m.ms})
		}
	}
	sort.Slice(out.Devices, func(i, j int) bool {
		a, c := out.Devices[i], out.Devices[j]
		if a.Plays != c.Plays {
			return a.Plays > c.Plays
		}
		if a.Name != c.Name {
			return a.Name < c.Name
		}
		return a.Type < c.Type
	})
	sort.Slice(out.Platforms, func(i, j int) bool {
		a, c := out.Platforms[i], out.Platforms[j]
		if a.Plays != c.Plays {
			return a.Plays > c.Plays
		}
		return a.Name < c.Name
	})
	return out, nil
}

// ---------- sessions ----------

// session is a run of visible plays [from, to) (indices into b.vis) where
// each play starts within the gap threshold of the previous one's end.
type session struct {
	from, to int
}

func (b *Backend) buildSessions() {
	gap := int64(b.settings.SessionGapMinutes) * 60_000
	b.sessions = b.sessions[:0]
	b.sessionByID = map[string]int{}
	plays := b.ds.Plays
	start := 0
	var end int64
	for k, i := range b.vis {
		p := plays[i]
		if k > 0 && p.StartMs-end > gap {
			b.sessions = append(b.sessions, session{start, k})
			start = k
		}
		end = max(end, p.EndMs())
		if k == 0 {
			end = p.EndMs()
		}
	}
	if len(b.vis) > 0 {
		b.sessions = append(b.sessions, session{start, len(b.vis)})
	}
	for n, s := range b.sessions {
		b.sessionByID[b.playID(b.vis[s.from])] = n
	}
}

func (b *Backend) sessionBounds(s session) (int64, int64) {
	start := b.ds.Plays[b.vis[s.from]].StartMs
	var end int64
	for _, i := range b.vis[s.from:s.to] {
		end = max(end, b.ds.Plays[i].EndMs())
	}
	return start, end
}

func (b *Backend) wireSession(s session) api.Session {
	start, end := b.sessionBounds(s)
	out := api.Session{ID: b.playID(b.vis[s.from]), StartedAt: start, EndedAt: end, DurationMs: end - start,
		Rows: int64(s.to - s.from), TopArtists: []api.ArtistRef{}}
	artistMs := map[int32]int64{}
	for _, i := range b.vis[s.from:s.to] {
		var m measure
		b.add(&m, i)
		out.MsPlayed += m.ms
		out.Plays += m.plays
		out.Skips += m.skips
		for _, a := range b.ds.Tracks[b.ds.Plays[i].Track].Artists {
			artistMs[a] += m.ms
		}
	}
	type am struct {
		a  int32
		ms int64
	}
	var list []am
	for a, ms := range artistMs {
		list = append(list, am{a, ms})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ms != list[j].ms {
			return list[i].ms > list[j].ms
		}
		return b.ds.Artists[list[i].a].ID < b.ds.Artists[list[j].a].ID
	})
	for _, x := range list[:min(3, len(list))] {
		out.TopArtists = append(out.TopArtists, b.artistRef(x.a))
	}
	return out
}

func (b *Backend) Sessions(ctx context.Context, spec query.Spec, p api.Page) (*api.Sessions, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, err := b.resolve(spec)
	if err != nil {
		return nil, err
	}
	type cand struct {
		s        session
		start    int64
		duration int64
	}
	var cands []cand
	lo, hi := cur.Start.UnixMilli(), cur.End.UnixMilli()
	for _, s := range b.sessions {
		start, end := b.sessionBounds(s)
		if start >= lo && start < hi {
			cands = append(cands, cand{s, start, end - start})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].duration != cands[j].duration {
			return cands[i].duration > cands[j].duration
		}
		return cands[i].start < cands[j].start
	})
	out := &api.Sessions{Range: wireRange(cur), GapMinutes: b.settings.SessionGapMinutes, Total: len(cands),
		Offset: p.Offset, Limit: p.Limit, Items: []api.Session{}}
	for _, c := range page(cands, p) {
		out.Items = append(out.Items, b.wireSession(c.s))
	}
	return out, nil
}

func (b *Backend) Session(ctx context.Context, id string) (*api.SessionDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n, ok := b.sessionByID[id]
	if !ok {
		return nil, fmt.Errorf("%w: session %q", api.ErrNotFound, id)
	}
	s := b.sessions[n]
	out := &api.SessionDetail{Session: b.wireSession(s), Plays: make([]api.Play, 0, s.to-s.from)}
	for _, i := range b.vis[s.from:s.to] {
		out.Plays = append(out.Plays, b.wirePlay(i))
	}
	return out, nil
}
