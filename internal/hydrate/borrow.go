package hydrate

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"sort"
	"time"

	"spotussi/internal/derive"
)

// Borrowing (D27): fallbacks find images and genres for provisional
// artists (export names), but a track Spotify has hydrated credits a real
// artist ID, which stays bare until Spotify hydrates the artist too. A
// real artist that is not hydrated borrows the provisional artist's
// image and genres when they are provably the same artist:
//
//   - exactly one real artist has that normalized name (two bands of one
//     name are two real artists, and neither borrows),
//   - exactly one provisional artist has it (true by construction: the
//     provisional ID is the normalized name), and
//   - they share a track: one the real artist is credited on in
//     track_artists whose export rows name the provisional artist.
//
// Each borrowed field gets a 'borrowed' provenance row with the lender's
// confidence, so a fallback that finds the real artist itself (oEmbed's
// exact image) wins, and a Spotify hydration replaces it (clearFallback).
// A field is only borrowed into a gap: an artist that has its own image
// or genres keeps them. Borrowed fields of an artist that stops
// qualifying (a second real artist of the name appears) are withdrawn.

// SourceBorrowed is the provenance source of a borrowed field.
const SourceBorrowed = "borrowed"

// borrowInterval is how often the fallback stages re-run Borrow while
// they make progress.
const borrowInterval = 2 * time.Minute

type lender struct {
	large, small string
	imageConf    float64
	genres       []string
	genreConf    float64
}

type borrower struct {
	id, name    string
	hydrated    bool
	hasImage    bool
	borrowedImg string // the borrowed image URL, if any
	borrowedGen bool
	ownGenres   bool // genres not borrowed
	genres      []string
}

// Borrow runs one borrowing pass over every real artist and reports how
// many artists it changed.
func (h *Hydrator) Borrow(ctx context.Context) (int, error) {
	h.borrowMu.Lock()
	defer h.borrowMu.Unlock()
	lenders, err := h.loadLenders(ctx)
	if err != nil {
		return 0, err
	}
	artists, err := h.loadBorrowers(ctx)
	if err != nil {
		return 0, err
	}
	names := map[string]int{}
	for _, a := range artists {
		names[derive.NormName(a.name)]++
	}
	changed := 0
	for _, a := range artists {
		l, ok := lenders[derive.ProvArtistID(a.name)]
		qualifies := ok && !a.hydrated && names[derive.NormName(a.name)] == 1
		if qualifies {
			if qualifies, err = h.sharesTrack(ctx, a.id, derive.NormName(a.name)); err != nil {
				return changed, err
			}
		}
		var n bool
		if qualifies {
			n, err = h.lend(ctx, a, l)
		} else if a.borrowedImg != "" || a.borrowedGen {
			n, err = h.withdraw(ctx, a)
		}
		if err != nil {
			return changed, err
		}
		if n {
			changed++
		}
	}
	if changed > 0 {
		h.changed(changed)
	}
	return changed, nil
}

// maybeBorrow runs Borrow if the last pass is older than borrowInterval.
func (h *Hydrator) maybeBorrow(ctx context.Context) {
	h.mu.Lock()
	due := h.now().Sub(h.lastBorrow) >= borrowInterval
	if due {
		h.lastBorrow = h.now()
	}
	h.mu.Unlock()
	if !due {
		return
	}
	if _, err := h.Borrow(ctx); err != nil && ctx.Err() == nil {
		h.mu.Lock()
		h.lastBorrow = time.Time{}
		h.mu.Unlock()
		slog.Warn("hydrate: borrow fallback metadata", "err", err)
	}
}

// loadLenders reads the provisional artists' images and genres with the
// confidence they were found at.
func (h *Hydrator) loadLenders(ctx context.Context) (map[string]*lender, error) {
	out := map[string]*lender{}
	get := func(id string) *lender {
		l := out[id]
		if l == nil {
			l = &lender{}
			out[id] = l
		}
		return l
	}
	rs, err := h.db.Read.QueryContext(ctx, `SELECT a.id, a.image_url, COALESCE(a.image_url_small, a.image_url), COALESCE(p.confidence, 0)
		FROM fallback_artists a
		LEFT JOIN catalog_provenance p ON p.entity_type = 'artist' AND p.entity_id = a.id AND p.field = 'image_url'
		WHERE a.image_url IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var id, large, small string
		var conf float64
		if err := rs.Scan(&id, &large, &small, &conf); err != nil {
			rs.Close()
			return nil, err
		}
		l := get(id)
		l.large, l.small, l.imageConf = large, small, conf
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	rs, err = h.db.Read.QueryContext(ctx, `SELECT g.artist_id, g.genre, COALESCE(p.confidence, 0) FROM artist_genres g
		LEFT JOIN catalog_provenance p ON p.entity_type = 'artist' AND p.entity_id = g.artist_id AND p.field = 'genres'
		WHERE g.artist_id LIKE '`+derive.ProvArtistPrefix+`%' ORDER BY g.artist_id, g.genre`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var id, g string
		var conf float64
		if err := rs.Scan(&id, &g, &conf); err != nil {
			return nil, err
		}
		l := get(id)
		l.genres, l.genreConf = append(l.genres, g), conf
	}
	return out, rs.Err()
}

// loadBorrowers reads every real artist with what it has of its own and
// what it borrowed.
func (h *Hydrator) loadBorrowers(ctx context.Context) ([]*borrower, error) {
	rs, err := h.db.Read.QueryContext(ctx, `SELECT a.id, a.name, a.hydrated_at IS NOT NULL, a.image_url IS NOT NULL,
			CASE WHEN pi.source = 'borrowed' THEN COALESCE(a.image_url, '') ELSE '' END,
			COALESCE(pg.source = 'borrowed', 0)
		FROM artists a
		LEFT JOIN catalog_provenance pi ON pi.entity_type = 'artist' AND pi.entity_id = a.id AND pi.field = 'image_url'
		LEFT JOIN catalog_provenance pg ON pg.entity_type = 'artist' AND pg.entity_id = a.id AND pg.field = 'genres'
		ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	var out []*borrower
	byID := map[string]*borrower{}
	for rs.Next() {
		b := &borrower{}
		if err := rs.Scan(&b.id, &b.name, &b.hydrated, &b.hasImage, &b.borrowedImg, &b.borrowedGen); err != nil {
			rs.Close()
			return nil, err
		}
		out = append(out, b)
		byID[b.id] = b
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	rs, err = h.db.Read.QueryContext(ctx, `SELECT g.artist_id, g.genre FROM artist_genres g JOIN artists a ON a.id = g.artist_id
		ORDER BY g.artist_id, g.genre`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var id, g string
		if err := rs.Scan(&id, &g); err != nil {
			return nil, err
		}
		if b := byID[id]; b != nil {
			b.genres = append(b.genres, g)
			b.ownGenres = !b.borrowedGen
		}
	}
	return out, rs.Err()
}

// sharesTrack reports whether a real artist is credited on a track whose
// export rows name the artist (norm: derive.NormName of the name; SQLite's
// lower and trim match it, both ASCII and spaces only).
func (h *Hydrator) sharesTrack(ctx context.Context, artistID, norm string) (bool, error) {
	var one int
	err := h.db.Read.QueryRowContext(ctx, `SELECT 1 FROM track_artists ta
		JOIN raw_export_rows r ON r.uri = 'spotify:track:' || ta.track_id
		WHERE ta.artist_id = ? AND r.artist_name IS NOT NULL AND lower(trim(r.artist_name, ' ')) = ? LIMIT 1`,
		artistID, norm).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// lend copies a lender's image and genres into the gaps of a qualifying
// real artist and reports whether anything changed.
func (h *Hydrator) lend(ctx context.Context, a *borrower, l *lender) (bool, error) {
	wantImg := l.large != "" && (!a.hasImage || (a.borrowedImg != "" && a.borrowedImg != l.large))
	wantGen := len(l.genres) > 0 && !a.ownGenres && !slices.Equal(sorted(a.genres), sorted(l.genres))
	if !wantImg && !wantGen {
		return false, nil
	}
	now := h.now().UnixMilli()
	changed := false
	err := h.tx(ctx, func(tx *sql.Tx) error {
		if wantImg {
			ok, err := claim(ctx, tx, "artist", a.id, "image_url", SourceBorrowed, l.imageConf, now)
			if err != nil {
				return err
			}
			if ok {
				res, err := tx.ExecContext(ctx, `UPDATE artists SET image_url = ?, image_url_small = ? WHERE id = ? AND hydrated_at IS NULL`,
					l.large, l.small, a.id)
				if err != nil {
					return err
				}
				n, _ := res.RowsAffected()
				changed = changed || n > 0
			}
		}
		if wantGen {
			ok, err := claim(ctx, tx, "artist", a.id, "genres", SourceBorrowed, l.genreConf, now)
			if err != nil || !ok {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM artist_genres WHERE artist_id = ?`, a.id); err != nil {
				return err
			}
			for _, g := range l.genres {
				if _, err := tx.ExecContext(ctx, `INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?) ON CONFLICT DO NOTHING`,
					a.id, g); err != nil {
					return err
				}
			}
			changed = true
		}
		return nil
	})
	return changed, err
}

// withdraw removes what an artist that no longer qualifies borrowed.
func (h *Hydrator) withdraw(ctx context.Context, a *borrower) (bool, error) {
	return true, h.tx(ctx, func(tx *sql.Tx) error {
		if a.borrowedImg != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE artists SET image_url = NULL, image_url_small = NULL
				WHERE id = ? AND hydrated_at IS NULL`, a.id); err != nil {
				return err
			}
		}
		if a.borrowedGen && !a.hydrated {
			if _, err := tx.ExecContext(ctx, `DELETE FROM artist_genres WHERE artist_id = ?`, a.id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM catalog_provenance WHERE entity_type = 'artist' AND entity_id = ? AND source = ?`,
			a.id, SourceBorrowed)
		return err
	})
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	sort.Strings(s)
	return s
}
