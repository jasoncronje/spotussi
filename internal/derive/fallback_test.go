package derive

import (
	"context"
	"testing"

	"spotussi/internal/db"
)

// Provisional artists and albums take what the fallback sources found
// for their names (D27); the track keeps its fallback duration.
func TestResolveFallbackMetadata(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ar, al := ProvArtistID("Some Artist"), ProvAlbumID("Some Artist", "Some Album")
	for _, q := range []string{
		`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x', 'cli', 'zip', 'done', 0)`,
		`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, track_name, artist_name, album_name, json)
			VALUES (1, x'01', 1000, 60000, 'track', 'spotify:track:t1', 'Song', 'Some Artist', 'Some Album', '{}')`,
		`INSERT INTO tracks (id, name, duration_ms, source) VALUES ('t1', 'Song', 213000, 'fallback')`,
		`INSERT INTO fallback_artists (id, name, image_url, updated_at) VALUES ('` + ar + `', 'Some Artist', 'https://img/artist', 1)`,
		`INSERT INTO fallback_albums (id, name, artist_name, album_type, release_date, total_tracks, image_url, updated_at)
			VALUES ('` + al + `', 'Some Album', 'Some Artist', 'album', '1987-11-12', 10, 'https://img/album', 1)`,
		`INSERT INTO artist_genres (artist_id, genre) VALUES ('` + ar + `', 'dance-pop'), ('` + ar + `', 'pop')`,
	} {
		if _, err := d.Write.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, filter := range []idFilter{nil, {"t1"}} { // full and incremental loads
		in, err := loadResolverInput(ctx, d.Read, filter, nil)
		if err != nil {
			t.Fatal(err)
		}
		e := Resolve(in, []string{"t1"})
		tr := e.Tracks[e.TrackByID["t1"]]
		if tr.DurationMs != 213000 || tr.Hydrated || len(tr.Artists) != 1 || tr.Album < 0 {
			t.Fatalf("filter %v: track %+v", filter, tr)
		}
		a := e.Artists[tr.Artists[0]]
		if !a.Provisional || a.ID != ar || a.ImageURL != "https://img/artist" || len(a.Genres) != 2 || len(tr.Genres) != 2 {
			t.Errorf("filter %v: artist %+v", filter, a)
		}
		alb := e.Albums[tr.Album]
		if !alb.Provisional || alb.ImageURL != "https://img/album" || alb.ReleaseYear != 1987 || alb.Type != "album" || alb.TotalTracks != 10 {
			t.Errorf("filter %v: album %+v", filter, alb)
		}
		if tr.Meta != MetaFallback || a.Meta != MetaFallback || alb.Meta != MetaFallback {
			t.Errorf("filter %v: meta track %d artist %d album %d, want fallback", filter, tr.Meta, a.Meta, alb.Meta)
		}
	}
}

// A track the poller seeded from a live track object is Spotify data even
// before its catalog lookup; an export-only track is known by name only.
func TestTrackMetaFromPollerSeed(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x', 'cli', 'zip', 'done', 0)`,
		`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, track_name, artist_name, album_name, json)
			VALUES (1, x'02', 1000, 60000, 'track', 'spotify:track:t2', 'Other', 'Some Artist', 'Some Album', '{}')`,
		`INSERT INTO artists (id, name) VALUES ('a1', 'Seeded Artist')`,
		`INSERT INTO tracks (id, name, duration_ms, source) VALUES ('t1', 'Seeded', 201000, 'poller')`,
		`INSERT INTO track_artists (track_id, artist_id, position) VALUES ('t1', 'a1', 0)`,
	} {
		if _, err := d.Write.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	in, err := loadResolverInput(ctx, d.Read, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := Resolve(in, []string{"t1", "t2"})
	if m := e.Tracks[e.TrackByID["t1"]].Meta; m != MetaSpotify {
		t.Errorf("poller-seeded track meta %d, want spotify", m)
	}
	if m := e.Tracks[e.TrackByID["t2"]].Meta; m != MetaPending {
		t.Errorf("export-only track meta %d, want pending", m)
	}
}
