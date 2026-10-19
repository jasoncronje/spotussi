package live

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// Entity pages for what the snapshot lacks but the catalog has: albums,
// songs and artists saved in the library but never played (D36). They
// open with zero stats instead of "not in your library".

func catalogMeta(hydrated bool, img, date string) derive.Meta {
	if hydrated || img != "" || date != "" {
		return derive.MetaSpotify
	}
	return derive.MetaPending
}

// catalogArtists returns the credited artists of a catalog album or
// track, in Spotify's order.
func (b *Backend) catalogArtists(ctx context.Context, x *q, table, key, id string) ([]api.ArtistRef, error) {
	rows, err := b.db.Read.QueryContext(ctx, `SELECT c.artist_id, COALESCE(a.name, c.artist_id), COALESCE(a.image_url, '')
		FROM `+table+` c LEFT JOIN artists a ON a.id = c.artist_id WHERE c.`+key+` = ? ORDER BY c.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.ArtistRef{}
	for rows.Next() {
		var aid, name, img string
		if err := rows.Scan(&aid, &name, &img); err != nil {
			return nil, err
		}
		if a, ok := x.e.ArtistByID[aid]; ok {
			out = append(out, x.artistRef(a))
		} else {
			out = append(out, api.ArtistRef{ID: aid, Name: name, ImageURL: strPtr(img)})
		}
	}
	return out, rows.Err()
}

func lites(as []api.ArtistRef) []api.ArtistLite {
	out := make([]api.ArtistLite, len(as))
	for i, a := range as {
		out[i] = api.ArtistLite{ID: a.ID, Name: a.Name}
	}
	return out
}

// catalogAlbumFull reads an album's catalog row; ok is false without one.
func (b *Backend) catalogAlbumFull(ctx context.Context, x *q, id string) (api.AlbumFull, derive.Meta, bool, error) {
	var name string
	var typ, img, date sql.NullString
	var total, hydrated sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT name, album_type, image_url, release_date, total_tracks, hydrated_at
		FROM albums WHERE id = ?`, id).Scan(&name, &typ, &img, &date, &total, &hydrated)
	if errors.Is(err, sql.ErrNoRows) {
		return api.AlbumFull{}, 0, false, nil
	}
	if err != nil {
		return api.AlbumFull{}, 0, false, err
	}
	artists, err := b.catalogArtists(ctx, x, "album_artists", "album_id", id)
	if err != nil {
		return api.AlbumFull{}, 0, false, err
	}
	full := api.AlbumFull{ID: id, Name: name, ImageURL: strPtr(img.String), AlbumType: strPtr(typ.String),
		ReleaseDate: strPtr(date.String), Artists: lites(artists)}
	if y := releaseYearOf(date.String); y > 0 {
		full.ReleaseYear = ptr(y)
	}
	if total.Int64 > 0 {
		full.TotalTracks = ptr(int(total.Int64))
	}
	return full, catalogMeta(hydrated.Valid, img.String, date.String), true, nil
}

func (b *Backend) unplayedAlbum(ctx context.Context, x *q, id string, spec query.Spec, metric query.Metric) (*api.AlbumDetail, error) {
	full, meta, ok, err := b.catalogAlbumFull(ctx, x, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: album %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kAlbum, -1, api.Scope{AlbumID: id})
	if err != nil {
		return nil, err
	}
	if full.Metadata, err = b.metadata(ctx, "album", id, meta); err != nil {
		return nil, err
	}
	return &api.AlbumDetail{Album: full, Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) unplayedTrack(ctx context.Context, x *q, id string, spec query.Spec, metric query.Metric) (*api.TrackDetail, error) {
	var name string
	var albumID sql.NullString
	var dur, explicit, hydrated sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT name, duration_ms, explicit, album_id, hydrated_at FROM tracks WHERE id = ?`, id).
		Scan(&name, &dur, &explicit, &albumID, &hydrated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: track %q", api.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kTrack, -1, api.Scope{TrackID: id})
	if err != nil {
		return nil, err
	}
	full := api.TrackFull{ID: id, Name: name}
	if dur.Int64 > 0 {
		full.DurationMs = ptr(dur.Int64)
	}
	if explicit.Valid {
		full.Explicit = ptr(explicit.Int64 != 0)
	}
	if full.Artists, err = b.catalogArtists(ctx, x, "track_artists", "track_id", id); err != nil {
		return nil, err
	}
	excluded := false
	for _, a := range full.Artists {
		if i, ok := x.e.ArtistByID[a.ID]; ok && x.ArtistExcluded(i) {
			excluded = true
		}
	}
	img := ""
	if albumID.Valid {
		if al, ok := x.e.AlbumByID[albumID.String]; ok {
			ref := x.albumRef(al)
			full.Album = &ref
		} else if af, _, ok, err := b.catalogAlbumFull(ctx, x, albumID.String); err != nil {
			return nil, err
		} else if ok {
			full.Album = &api.AlbumRef{ID: af.ID, Name: af.Name, ImageURL: af.ImageURL, ReleaseYear: af.ReleaseYear, Artists: af.Artists}
		}
		if full.Album != nil {
			full.ImageURL = full.Album.ImageURL
			if full.ImageURL != nil {
				img = *full.ImageURL
			}
		}
	}
	if full.Metadata, err = b.metadata(ctx, "track", id, catalogMeta(hydrated.Valid, img, "")); err != nil {
		return nil, err
	}
	saved, err := b.savedAt(ctx, id)
	if err != nil {
		return nil, err
	}
	return &api.TrackDetail{Track: full, Excluded: excluded, SavedAt: saved, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) unplayedArtist(ctx context.Context, x *q, id string, spec query.Spec, metric query.Metric) (*api.ArtistDetail, error) {
	var name string
	var img sql.NullString
	var hydrated sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT name, image_url, hydrated_at FROM artists WHERE id = ?`, id).
		Scan(&name, &img, &hydrated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: artist %q", api.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kArtist, -1, api.Scope{ArtistID: id})
	if err != nil {
		return nil, err
	}
	full := api.ArtistFull{ID: id, Name: name, ImageURL: strPtr(img.String), Genres: []api.GenreRef{}}
	// Only genres some played artist has: others have no genre page.
	rows, err := b.db.Read.QueryContext(ctx, `SELECT genre FROM artist_genres WHERE artist_id = ? ORDER BY genre`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		if i, ok := x.e.GenreByID[derive.GenreID(g)]; ok && x.e.Genres[i].Name == g {
			full.Genres = append(full.Genres, x.genreRef(i))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if full.Metadata, err = b.metadata(ctx, "artist", id, catalogMeta(hydrated.Valid, img.String, "")); err != nil {
		return nil, err
	}
	return &api.ArtistDetail{Artist: full, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}
