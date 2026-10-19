package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"spotussi/internal/db"
)

// Job is one row of the imports table.
type Job struct {
	ID            int64
	Filename      string
	Origin        string // upload, cli
	Kind          Kind
	State         string // queued, running, done, failed
	Files         int
	FilesDone     int
	BytesTotal    *int64
	BytesDone     int64
	RowsTotal     *int64
	RowsDone      int64
	RowsNew       int64
	RowsDuplicate int64
	CreatedAt     int64
	StartedAt     *int64
	FinishedAt    *int64
	ErrorCode     string
	Error         string
}

// Job states.
const (
	StateQueued  = "queued"
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// ErrNotFound is returned for an unknown job ID.
var ErrNotFound = errors.New("no such import")

const jobColumns = `id, filename, origin, kind, state, files, files_done, bytes_total, bytes_done, rows_total,
	rows_done, rows_new, rows_duplicate, created_at, started_at, finished_at, error_code, error`

func scanJob(sc interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var bytesTotal, rowsTotal, started, finished sql.NullInt64
	var code, msg sql.NullString
	err := sc.Scan(&j.ID, &j.Filename, &j.Origin, &j.Kind, &j.State, &j.Files, &j.FilesDone, &bytesTotal, &j.BytesDone,
		&rowsTotal, &j.RowsDone, &j.RowsNew, &j.RowsDuplicate, &j.CreatedAt, &started, &finished, &code, &msg)
	ptr := func(n sql.NullInt64) *int64 {
		if !n.Valid {
			return nil
		}
		v := n.Int64
		return &v
	}
	j.BytesTotal, j.RowsTotal, j.StartedAt, j.FinishedAt = ptr(bytesTotal), ptr(rowsTotal), ptr(started), ptr(finished)
	j.ErrorCode, j.Error = code.String, msg.String
	return j, err
}

// Get reads job id.
func Get(ctx context.Context, d *db.DB, id int64) (*Job, error) {
	j, err := scanJob(d.Read.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM imports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// List returns up to limit jobs, newest first.
func List(ctx context.Context, d *db.DB, limit int) ([]Job, error) {
	rs, err := d.Read.QueryContext(ctx, `SELECT `+jobColumns+` FROM imports ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []Job{}
	for rs.Next() {
		j, err := scanJob(rs)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rs.Err()
}

func create(ctx context.Context, d *db.DB, filename, origin string, kind Kind, state, sourcePath string, now time.Time) (int64, error) {
	var src any
	if sourcePath != "" {
		src = sourcePath
	}
	res, err := d.Write.ExecContext(ctx, `INSERT INTO imports (filename, origin, kind, state, source_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, filename, origin, string(kind), state, src, now.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("create import: %w", err)
	}
	return res.LastInsertId()
}

// ImportPath imports the zip, JSON file or directory at p synchronously
// (the CLI). It records a job like an upload does and reports progress
// after every batch. The returned error is also recorded on the job.
func ImportPath(ctx context.Context, d *db.DB, p string, progress func(Stats)) (*Job, Stats, error) {
	kind, err := SniffPath(p)
	if err != nil {
		return nil, Stats{}, err
	}
	abs, _ := filepath.Abs(p)
	name := filepath.Base(abs)
	id, err := create(ctx, d, name, "cli", kind, StateRunning, "", time.Now())
	if err != nil {
		return nil, Stats{}, err
	}
	st, runErr := execute(ctx, d, id, p, kind, name, time.Now, progress)
	j, err := Get(context.WithoutCancel(ctx), d, id)
	if err != nil {
		return nil, st, errors.Join(runErr, err)
	}
	return j, st, runErr
}

// Service runs uploaded imports in the server, one at a time, in upload
// order. Queued jobs live in the imports table with the path of their
// uploaded file, so a restart picks them up again; a job that was running
// when the server stopped is failed as interrupted (re-importing finishes
// it, since inserts are idempotent).
type Service struct {
	db         *db.DB
	dir        string
	now        func() time.Time
	onFinished func()
	wake       chan struct{}
	done       func()
}

// Options configure a Service.
type Options struct {
	DB *db.DB
	// Dir holds uploaded files until their import finishes (under
	// DATA_DIR, so uploads never sit in memory).
	Dir string
	// Now defaults to time.Now.
	Now func() time.Time
	// OnFinished is called after every job that added rows, even a
	// failed one (its committed rows are kept), e.g. to queue the new
	// tracks for hydration (M4). It must not block.
	OnFinished func()
	// OnNewRows, if set, is called after a job stores new rows (also a
	// failed one: its rows are committed batch by batch), so the
	// derivation engine rebuilds.
	OnNewRows func()
}

// NewService prepares the upload directory and recovers jobs left by a
// previous process.
func NewService(ctx context.Context, o Options) (*Service, error) {
	s := &Service{db: o.DB, dir: o.Dir, now: o.Now, onFinished: o.OnFinished, wake: make(chan struct{}, 1), done: o.OnNewRows}
	if s.now == nil {
		s.now = time.Now
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create import dir: %w", err)
	}
	if err := s.recover(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir is where the HTTP layer writes uploads.
func (s *Service) Dir() string { return s.dir }

func (s *Service) recover(ctx context.Context) error {
	msg := "the server stopped during this import; import the file again to finish it (rows already imported are kept)"
	if _, err := s.db.Write.ExecContext(ctx, `UPDATE imports SET state = 'failed', source_path = NULL, finished_at = ?,
		error_code = ?, error = ? WHERE state = 'running' AND origin = 'upload'`, s.now().UnixMilli(), CodeInterrupted, msg); err != nil {
		return err
	}
	keep := map[string]bool{}
	rs, err := s.db.Write.QueryContext(ctx, `SELECT id, source_path FROM imports WHERE state = 'queued'`)
	if err != nil {
		return err
	}
	var lost []int64
	for rs.Next() {
		var id int64
		var p sql.NullString
		if err := rs.Scan(&id, &p); err != nil {
			rs.Close()
			return err
		}
		if _, err := os.Stat(p.String); p.Valid && err == nil {
			keep[filepath.Clean(p.String)] = true
		} else {
			lost = append(lost, id)
		}
	}
	rs.Close()
	for _, id := range lost {
		if _, err := s.db.Write.ExecContext(ctx, `UPDATE imports SET state = 'failed', source_path = NULL, finished_at = ?,
			error_code = ?, error = ? WHERE id = ?`, s.now().UnixMilli(), CodeInterrupted, "the uploaded file is gone; upload it again", id); err != nil {
			return err
		}
	}
	// Leftovers of interrupted uploads and finished jobs.
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		p := filepath.Join(s.dir, e.Name())
		if !keep[p] && strings.HasPrefix(e.Name(), UploadPrefix) {
			os.Remove(p)
		}
	}
	return nil
}

// UploadPrefix starts the name of every uploaded file in Dir.
const UploadPrefix = "upload-"

// Submit queues an import of the uploaded file at path (inside Dir; the
// service deletes it when the job ends).
func (s *Service) Submit(ctx context.Context, path, filename string, kind Kind) (*Job, error) {
	id, err := create(ctx, s.db, filename, "upload", kind, StateQueued, path, s.now())
	if err != nil {
		return nil, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return Get(ctx, s.db, id)
}

// Job reads one job.
func (s *Service) Job(ctx context.Context, id int64) (*Job, error) { return Get(ctx, s.db, id) }

// Jobs lists up to limit jobs, newest first.
func (s *Service) Jobs(ctx context.Context, limit int) ([]Job, error) { return List(ctx, s.db, limit) }

// Run processes queued jobs until ctx ends.
func (s *Service) Run(ctx context.Context) {
	for {
		var id int64
		var path, filename, kind string
		err := s.db.Read.QueryRowContext(ctx, `SELECT id, source_path, filename, kind FROM imports
			WHERE state = 'queued' ORDER BY id LIMIT 1`).Scan(&id, &path, &filename, &kind)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			slog.Error("import: read queue", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		s.process(ctx, id, path, filename, Kind(kind))
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Service) process(ctx context.Context, id int64, path, filename string, kind Kind) {
	start := time.Now()
	slog.Info("import started", "id", id, "kind", kind)
	st, err := execute(ctx, s.db, id, path, kind, filename, s.now, nil)
	os.Remove(path)
	if st.RowsNew > 0 && s.onFinished != nil {
		s.onFinished()
	}
	if st.RowsNew > 0 && s.done != nil {
		s.done()
	}
	if err != nil {
		code, _ := errorCode(err)
		slog.Warn("import failed", "id", id, "code", code, "rows", st.RowsDone, "new", st.RowsNew)
		return
	}
	slog.Info("import done", "id", id, "files", st.Files, "rows", st.RowsDone, "new", st.RowsNew,
		"duplicate", st.RowsDuplicate, "took", time.Since(start).Round(time.Millisecond))
}
