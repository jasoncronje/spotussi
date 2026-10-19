package live

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// ---------- devices ----------

func (b *Backend) Devices(ctx context.Context, spec query.Spec) (*api.Devices, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, err := x.resolve(spec)
	if err != nil {
		return nil, err
	}
	dev := make([]measure, len(x.e.Devices))
	plat := make([]measure, len(x.e.Platforms))
	for _, i := range x.inRange(cur) {
		if d := x.Device[i]; d >= 0 {
			x.add(&dev[d], i)
		}
		if p := x.Platform[i]; p >= 0 {
			x.add(&plat[p], i)
		}
	}
	out := &api.Devices{Range: wireRange(cur), Devices: []api.DeviceUsage{}, Platforms: []api.PlatformUsage{}}
	for d, m := range dev {
		if m.ms > 0 || m.plays > 0 {
			v := x.e.Devices[d]
			out.Devices = append(out.Devices, api.DeviceUsage{Name: v.Name, Type: v.Type, Plays: m.plays, MsPlayed: m.ms})
		}
	}
	for p, m := range plat {
		if m.ms > 0 || m.plays > 0 {
			out.Platforms = append(out.Platforms, api.PlatformUsage{Name: x.e.Platforms[p], Plays: m.plays, MsPlayed: m.ms})
		}
	}
	slices.SortFunc(out.Devices, func(a, c api.DeviceUsage) int {
		return cmp.Or(cmp.Compare(c.Plays, a.Plays), cmp.Compare(a.Name, c.Name), cmp.Compare(a.Type, c.Type))
	})
	slices.SortFunc(out.Platforms, func(a, c api.PlatformUsage) int {
		return cmp.Or(cmp.Compare(c.Plays, a.Plays), cmp.Compare(a.Name, c.Name))
	})
	return out, nil
}

// ---------- sessions ----------

func (x *q) sessionID(s derive.Session) string { return strconv.FormatInt(x.ID[x.Vis[s.From]], 10) }

func (x *q) wireSession(s derive.Session) api.Session {
	out := api.Session{ID: x.sessionID(s), StartedAt: s.Start, EndedAt: s.End, DurationMs: s.End - s.Start,
		Rows: int64(s.To - s.From), TopArtists: []api.ArtistRef{}}
	artistMs := map[int32]int64{}
	for _, i := range x.Vis[s.From:s.To] {
		var m measure
		x.add(&m, i)
		out.MsPlayed += m.ms
		out.Plays += m.plays
		out.Skips += m.skips
		for _, a := range x.e.Tracks[x.Track[i]].Artists {
			artistMs[a] += m.ms
		}
	}
	type am struct {
		a  int32
		ms int64
	}
	list := make([]am, 0, len(artistMs))
	for a, ms := range artistMs {
		list = append(list, am{a, ms})
	}
	slices.SortFunc(list, func(p, r am) int {
		return cmp.Or(cmp.Compare(r.ms, p.ms), cmp.Compare(x.e.Artists[p.a].ID, x.e.Artists[r.a].ID))
	})
	for _, v := range list[:min(3, len(list))] {
		out.TopArtists = append(out.TopArtists, x.artistRef(v.a))
	}
	return out
}

func (b *Backend) Sessions(ctx context.Context, spec query.Spec, p api.Page) (*api.Sessions, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, err := x.resolve(spec)
	if err != nil {
		return nil, err
	}
	lo, hi := cur.Start.UnixMilli(), cur.End.UnixMilli()
	var cands []derive.Session
	for _, s := range x.Sessions {
		if s.Start >= lo && s.Start < hi {
			cands = append(cands, s)
		}
	}
	slices.SortFunc(cands, func(a, c derive.Session) int {
		return cmp.Or(cmp.Compare(c.End-c.Start, a.End-a.Start), cmp.Compare(a.Start, c.Start))
	})
	out := &api.Sessions{Range: wireRange(cur), GapMinutes: x.View.Settings.SessionGapMinutes, Total: len(cands),
		Offset: p.Offset, Limit: p.Limit, Items: []api.Session{}}
	for _, s := range page(cands, p) {
		out.Items = append(out.Items, x.wireSession(s))
	}
	return out, nil
}

func (b *Backend) Session(ctx context.Context, id string) (*api.SessionDetail, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	pid, err := strconv.ParseInt(id, 10, 64)
	n, ok := x.SessionOf[pid]
	if err != nil || !ok {
		return nil, fmt.Errorf("%w: session %q", api.ErrNotFound, id)
	}
	s := x.Sessions[n]
	out := &api.SessionDetail{Session: x.wireSession(s), Plays: make([]api.Play, 0, s.To-s.From)}
	for _, i := range x.Vis[s.From:s.To] {
		out.Plays = append(out.Plays, x.wirePlay(i))
	}
	return out, nil
}
