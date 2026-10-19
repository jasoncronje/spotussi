package live

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"spotussi/internal/api"
	"spotussi/internal/derive"
)

// History serves the play log, newest first, from the derived snapshot.
//
// The cursor names the last play of the previous page as
// "<start base36>~<id base36>". Play IDs survive incremental updates, so
// a page boundary holds while live plays arrive; if the play it names
// has gone (a rebuild re-merged it), the next page resumes after its
// start time.
func (b *Backend) History(ctx context.Context, p api.HistoryParams) (*api.History, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, err := x.resolve(p.Range)
	if err != nil {
		return nil, err
	}
	sc, err := x.scope(p.Scope)
	if err != nil {
		return nil, err
	}
	plays := x.inRange(cur)
	before := len(plays) // only positions below this are on the page
	if p.Cursor != "" {
		if before, err = x.cursorPos(plays, p.Cursor); err != nil {
			return nil, err
		}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	match := func(i int32) bool {
		f := x.Flags[i]
		switch {
		case !sc.has(x, i):
			return false
		case p.Skipped != nil && (f&derive.FSkipped != 0) != *p.Skipped:
			return false
		case p.Shuffle != nil && (f&derive.FShuffleKnown == 0 || (f&derive.FShuffle != 0) != *p.Shuffle):
			return false
		case p.Counted != nil && x.Counted[i] != *p.Counted:
			return false
		case p.Device != "" && (x.Device[i] < 0 || x.e.Devices[x.Device[i]].Name != p.Device):
			return false
		case p.Platform != "" && (x.Platform[i] < 0 || x.e.Platforms[x.Platform[i]] != p.Platform):
			return false
		}
		return true
	}
	out := &api.History{Range: wireRange(cur), Items: []api.Play{}}
	more := false
	for k := len(plays) - 1; k >= 0; k-- {
		i := plays[k]
		if !match(i) {
			continue
		}
		out.Total++
		if k >= before {
			continue
		}
		if len(out.Items) < limit {
			out.Items = append(out.Items, x.wirePlay(i))
		} else {
			more = true
		}
	}
	if more {
		last := out.Items[len(out.Items)-1]
		id, _ := strconv.ParseInt(last.ID, 10, 64)
		c := strconv.FormatInt(last.PlayedAt, 36) + "~" + strconv.FormatInt(id, 36)
		out.NextCursor = &c
	}
	return out, nil
}

// cursorPos finds the position in plays (sorted by start) below which
// the next page starts.
func (x *q) cursorPos(plays []int32, c string) (int, error) {
	s, id, ok := strings.Cut(c, "~")
	start, err1 := strconv.ParseInt(s, 36, 64)
	pid, err2 := strconv.ParseInt(id, 36, 64)
	if !ok || err1 != nil || err2 != nil {
		return 0, fmt.Errorf("%w: invalid cursor", api.ErrBadRequest)
	}
	k := 0
	for k < len(plays) && x.Start[plays[k]] < start {
		k++
	}
	for j := k; j < len(plays) && x.Start[plays[j]] == start; j++ {
		if x.ID[plays[j]] == pid {
			return j, nil
		}
	}
	return k, nil
}
