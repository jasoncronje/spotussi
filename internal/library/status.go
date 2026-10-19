package library

import (
	"context"
	"database/sql"
	"slices"

	"spotussi/internal/spotify"
)

// Status is library sync's progress, for /system/status.
type Status struct {
	State      string
	LastSyncAt int64 // finished, without error; 0: never
	LastFullAt int64
	NextSyncAt int64
	ResumesAt  int64 // quota_exceeded: when the bucket resets
	Progress   float64
	LastError  string
	LastErrAt  int64
	// Lists: tracks, albums, playlists.
	Lists map[string]ListStatus
	// Counts of what is in the library now.
	SavedTracks, SavedAlbums, Playlists, PlaylistsWithItems int
}

// ListStatus is one list's state (ok, pending, needs_scope, error).
type ListStatus struct {
	State     string
	LastError string
}

// Status reports the service's state, with counts from the database.
func (s *Service) Status(ctx context.Context) (Status, error) {
	st, err := ReadStatus(ctx, s.db.Read, nil)
	if err != nil {
		return st, err
	}
	var scopes []string
	connected := false
	if s.sp != nil {
		if cs, err := s.sp.Status(ctx); err == nil && cs.State == spotify.StateConnected {
			scopes, connected = cs.Scopes, true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.State = s.state
	if !connected {
		st.State = StateNotConnected
	}
	if !s.nextAt.IsZero() && st.State != StateSyncing {
		st.NextSyncAt = s.nextAt.UnixMilli()
	}
	if st.State == StateQuotaExceeded && !s.resumesAt.IsZero() {
		st.ResumesAt = s.resumesAt.UnixMilli()
	}
	if st.State == StateSyncing {
		st.Progress = s.progress
	}
	st.LastError = s.lastErr
	if !s.lastErrAt.IsZero() {
		st.LastErrAt = s.lastErrAt.UnixMilli()
	}
	for l, ls := range s.lists {
		st.Lists[l] = ListStatus{State: ls.state, LastError: ls.err}
	}
	if connected && (!slices.Contains(scopes, spotify.ScopeLibrary) || s.scopeDenied) {
		for _, l := range []string{ListTracks, ListAlbums} {
			st.Lists[l] = ListStatus{State: ListNeedsScope}
		}
	}
	return st, nil
}

// ReadStatus reads what the database knows (counts, last syncs) for an
// instance where sync is not running (state disabled); scopes, when
// given, mark the library lists that need a scope.
func ReadStatus(ctx context.Context, q *sql.DB, scopes []string) (Status, error) {
	st := Status{State: StateDisabled, Lists: map[string]ListStatus{}}
	var lastSync, lastFull sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT MAX(finished_at) FROM library_syncs WHERE error IS NULL AND finished_at IS NOT NULL),
		(SELECT MAX(finished_at) FROM library_syncs WHERE kind = 'full' AND error IS NULL AND finished_at IS NOT NULL),
		(SELECT COUNT(*) FROM library_spans WHERE list = 'tracks' AND gone_sync IS NULL),
		(SELECT COUNT(*) FROM library_spans WHERE list = 'albums' AND gone_sync IS NULL),
		(SELECT COUNT(*) FROM library_playlists WHERE gone_sync IS NULL),
		(SELECT COUNT(*) FROM library_playlists WHERE gone_sync IS NULL AND items_available = 1)`).
		Scan(&lastSync, &lastFull, &st.SavedTracks, &st.SavedAlbums, &st.Playlists, &st.PlaylistsWithItems)
	if err != nil {
		return st, err
	}
	st.LastSyncAt, st.LastFullAt = lastSync.Int64, lastFull.Int64
	// A list never walked or checked is pending.
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT list FROM library_walks WHERE list IN ('tracks', 'albums', 'playlists')`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	done := map[string]bool{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return st, err
		}
		done[l] = true
	}
	for _, l := range []string{ListTracks, ListAlbums, ListPlaylists} {
		st.Lists[l] = ListStatus{State: ListPending}
		if done[l] {
			st.Lists[l] = ListStatus{State: ListOK}
		}
	}
	if scopes != nil && !slices.Contains(scopes, spotify.ScopeLibrary) {
		st.Lists[ListTracks] = ListStatus{State: ListNeedsScope}
		st.Lists[ListAlbums] = ListStatus{State: ListNeedsScope}
	}
	return st, rows.Err()
}

