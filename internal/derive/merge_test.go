package derive

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"spotussi/internal/db"
)

const min_ = 60_000

func summary(plays []Play) []string {
	var out []string
	for _, p := range plays {
		out = append(out, fmt.Sprintf("%s@%d-%d ms=%d src=%d est=%t skip=%t contra=%t ctx=%s plat=%s",
			p.TrackID, p.StartedMs, p.EndedMs, p.MsPlayed, p.Sources, p.Estimated, p.Skipped, p.Contradicted, p.ContextURI, p.Platform))
	}
	return out
}

func check(t *testing.T, got []Play, want ...string) {
	t.Helper()
	if s := summary(got); !slices.Equal(s, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(s, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestMergeSources(t *testing.T) {
	// A poller play confirmed by recently-played and the export: the
	// export's measurements and platform win, the poller's context stays.
	got := Merge(MergeInput{
		Poller: []PollerRow{{ID: 1, TrackID: "a", DurationMs: 200_000, StartedMs: 0, EndedMs: 200_000, MsPlayed: 198_000,
			ContextURI: "spotify:album:x"}},
		Recent: []RecentRow{{PlayedAtMs: 200_400, TrackID: "a", DurationMs: 200_000}},
		Export: []ExportRow{{ID: 7, TsMs: 201_000, MsPlayed: 199_000, TrackID: "a", Platform: "ios", Skipped: False}},
	})
	check(t, got, "a@0-200000 ms=199000 src=7 est=false skip=false contra=false ctx=spotify:album:x plat=ios")

	// Recently-played alone: estimated as the track length, placed by
	// the calibration verdict.
	in := MergeInput{Recent: []RecentRow{{PlayedAtMs: 10 * min_, TrackID: "b", DurationMs: 3 * min_}}}
	check(t, Merge(in), "b@420000-600000 ms=180000 src=4 est=true skip=false contra=false ctx= plat=")
	in.PlayedAtIsStart = true
	check(t, Merge(in), "b@600000-780000 ms=180000 src=4 est=true skip=false contra=false ctx= plat=")

	// Inside the export's span, a poller play the export lacks is
	// contradicted; an old export with skipped null takes fwdbtn as a
	// skip.
	got = Merge(MergeInput{
		Poller:     []PollerRow{{ID: 1, TrackID: "c", StartedMs: 5 * min_, EndedMs: 8 * min_, MsPlayed: 3 * min_}},
		Export:     []ExportRow{{ID: 1, TsMs: 2 * min_, MsPlayed: 30_000, TrackID: "d", ReasonEnd: "fwdbtn"}},
		ExportSpan: [2]int64{0, 10 * min_},
	})
	check(t, got,
		"d@90000-120000 ms=30000 src=1 est=false skip=true contra=false ctx= plat=",
		"c@300000-480000 ms=180000 src=2 est=false skip=false contra=true ctx= plat=")
}

// A track on repeat: an unpolled play, then a polled one. Each
// recently-played item must stay with its own play, though both lie in
// the polled play's matching window.
func TestMergeRepeatKeepsItems(t *testing.T) {
	got := Merge(MergeInput{
		Poller: []PollerRow{{ID: 1, TrackID: "a", DurationMs: 200_000, StartedMs: 200_500, EndedMs: 400_500, MsPlayed: 200_000}},
		Recent: []RecentRow{
			{PlayedAtMs: 200_000, TrackID: "a", DurationMs: 200_000}, // the unpolled play's
			{PlayedAtMs: 400_500, TrackID: "a", DurationMs: 200_000}, // the polled play's
		},
	})
	check(t, got,
		"a@0-200000 ms=200000 src=4 est=true skip=false contra=false ctx= plat=",
		"a@200500-400500 ms=200000 src=6 est=false skip=false contra=false ctx= plat=")
}

// A short skip right after an unpolled play of the same track cannot
// take that play's recently-played item (Spotify lists only plays of
// 30 s or more), nor its export row.
func TestMergeShortSkipAfterUnpolledPlay(t *testing.T) {
	got := Merge(MergeInput{
		Poller: []PollerRow{{ID: 1, TrackID: "a", DurationMs: 194_000, StartedMs: 195_000, EndedMs: 211_000, MsPlayed: 16_000, Skipped: true}},
		Recent: []RecentRow{{PlayedAtMs: 193_000, TrackID: "a", DurationMs: 194_000}},
		Export: []ExportRow{
			{ID: 1, TsMs: 193_000, MsPlayed: 193_000, TrackID: "a", Skipped: False, Platform: "ios"},
			{ID: 2, TsMs: 211_000, MsPlayed: 16_000, TrackID: "a", Skipped: True, Platform: "ios"},
		},
	})
	check(t, got,
		"a@0-193000 ms=193000 src=5 est=false skip=false contra=false ctx= plat=ios",
		"a@195000-211000 ms=16000 src=3 est=false skip=true contra=false ctx= plat=ios")
}

// The swap tables a rebuild writes must have exactly the migration's
// columns, or a rebuild would silently change the schema.
func TestSwapDDLMatchesMigration(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	cols := func(table string) string {
		rs, err := d.Read.QueryContext(ctx, `SELECT name, type, "notnull", COALESCE(dflt_value, ''), pk FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		var out []string
		for rs.Next() {
			var name, typ, dflt string
			var notnull, pk int
			if err := rs.Scan(&name, &typ, &notnull, &dflt, &pk); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprint(name, typ, notnull, dflt, pk))
		}
		return strings.Join(out, ",")
	}
	for _, ddl := range []string{fmt.Sprintf(playsDDL, "_new"), fmt.Sprintf(sessionsDDL, "_new")} {
		if _, err := d.Write.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"plays", "sessions"} {
		if a, b := cols(table), cols(table+"_new"); a != b || a == "" {
			t.Errorf("%s: migration %s, rebuild %s", table, a, b)
		}
	}
	var n int
	d.Read.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('plays')`).Scan(&n)
	if n != len(strings.Split(playsColumns, ",")) {
		t.Errorf("plays has %d columns, insert lists %d", n, len(strings.Split(playsColumns, ",")))
	}
}

// An export row naming a track Spotify has since relinked merges with the
// live play of the relinked track (tracks.linked_from, from hydration).
func TestReadExportRelinked(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x', 'cli', 'zip', 'done', 0)`,
		`INSERT INTO tracks (id, name, linked_from, source, hydrated_at) VALUES ('new', 'Song', 'old', 'poller', 1)`,
		`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, json)
			VALUES (1, x'01', 200000, 180000, 'track', 'spotify:track:old', '{}'),
			       (1, x'02', 900000, 180000, 'track', 'spotify:track:other', '{}')`,
	} {
		if _, err := d.Write.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]ExportNames{}
	rows, err := readExport(ctx, d.Read, rawWindow{}, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].TrackID != "new" || rows[1].TrackID != "other" {
		t.Fatalf("track IDs: %+v", rows)
	}
	if _, ok := names["new"]; !ok {
		t.Errorf("names keyed by the original ID: %v", names)
	}
	in, err := loadResolverInput(ctx, d.Read, idFilter{"new"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := in.Export["new"]; !ok {
		t.Errorf("filtered export names miss the relinked track: %v", in.Export)
	}
	plays := Merge(MergeInput{Export: rows, Poller: []PollerRow{{ID: 1, TrackID: "new", StartedMs: 20000, EndedMs: 201000, MsPlayed: 180000}}})
	if len(plays) != 2 || plays[0].Sources != SrcExport|SrcPoller {
		t.Errorf("not merged: %s", summary(plays))
	}
}
