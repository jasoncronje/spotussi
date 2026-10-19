// Package importer ingests the Extended Streaming History export (D10,
// D23) into raw_export_rows, the export's part of the raw layer (D3).
//
// An import reads a zip package, a single history JSON file, or (CLI) an
// unpacked directory. Every Streaming_History_{Audio,Video}_*.json file is
// parsed as a stream, one array element at a time, and every row is
// stored verbatim with extracted columns. Rows are inserted in
// transactions of a few thousand with ON CONFLICT DO NOTHING on a hash of
// (ts, uri, ms_played), so re-importing the same or an overlapping export
// adds only what is new. Nothing here reads more than one row and one
// batch into memory, whatever the export's size.
//
// The server runs uploads through a Service (one at a time, queued in the
// imports table); the CLI calls ImportPath synchronously.
package importer

import (
	"archive/zip"
	"compress/flate"
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"time"

	"spotussi/internal/db"
)

// Error codes reported in imports.error_code (and ImportJob.error_code).
const (
	CodeUnsupportedFile = "unsupported_file" // neither a zip nor JSON
	CodeNoHistoryFiles  = "no_history_files" // a zip/dir without Extended history files
	CodeBasicExport     = "basic_export"     // the basic Account Data export (D10)
	CodeMalformedFile   = "malformed_file"   // broken zip, invalid JSON or rows
	CodeInterrupted     = "interrupted"      // the server stopped mid-import
	CodeInternal        = "internal"         // database or I/O failure
)

// Error is an import failure with a machine-readable code. Messages never
// contain row contents (DF4).
type Error struct {
	Code, Msg string
}

func (e *Error) Error() string { return e.Msg }

// errorCode maps any import error to a code and a message safe to show.
func errorCode(err error) (code, msg string) {
	var ie *Error
	switch {
	case errors.As(err, &ie):
		return ie.Code, ie.Msg
	case errors.Is(err, context.Canceled):
		return CodeInterrupted, "the import was interrupted; import the file again to finish it (rows already imported are kept)"
	}
	return CodeInternal, err.Error()
}

// BatchSize is the number of rows inserted per transaction.
const BatchSize = 5000

// Stats are an import's counters.
type Stats struct {
	Files, FilesDone int
	BytesTotal       int64
	BytesDone        int64
	RowsDone         int64            // rows read
	RowsNew          int64            // rows inserted
	RowsDuplicate    int64            // rows already present
	NewByType        map[string]int64 // rows inserted per content type
}

const insertSQL = `INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri,
	track_name, artist_name, album_name, reason_start, reason_end, shuffle, skipped, offline, incognito,
	platform, conn_country, video, json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (row_hash) DO NOTHING`

// pending is a parsed row waiting for its batch.
type pending struct {
	row
	video bool
	json  string
}

// importer runs one job.
type importer struct {
	db       *db.DB
	jobID    int64
	st       Stats
	batch    []pending
	progress func(Stats)
	now      func() time.Time
}

var unmarshalOpts = json.JoinOptions(jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true))

// run imports every file of src into job jobID.
func (im *importer) run(ctx context.Context, src *source) error {
	im.st.Files = len(src.files)
	im.st.NewByType = map[string]int64{}
	for _, f := range src.files {
		if f.size > 0 {
			im.st.BytesTotal += f.size
		}
	}
	if err := im.started(ctx); err != nil {
		return err
	}
	for _, f := range src.files {
		base := im.st.BytesDone
		if err := im.file(ctx, f, base); err != nil {
			// Keep what parsed before the failure: inserts are
			// idempotent, so a later fixed import completes it.
			if ferr := im.flush(ctx); ferr != nil && !errors.Is(err, context.Canceled) {
				return ferr
			}
			return err
		}
		im.st.FilesDone++
		im.st.BytesDone = base + max(f.size, 0)
	}
	return im.flush(ctx)
}

// file streams one history file.
func (im *importer) file(ctx context.Context, f file, base int64) error {
	rc, err := f.open()
	if err != nil {
		return &Error{Code: CodeMalformedFile, Msg: fmt.Sprintf("%s: cannot be read from the archive", f.name)}
	}
	defer rc.Close()
	cnt := &counter{r: rc}
	dec := jsontext.NewDecoder(bomSkipper(cnt), jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true))
	malformed := func(err error) error {
		var se *jsontext.SyntacticError
		if errors.As(err, &se) {
			return &Error{Code: CodeMalformedFile, Msg: fmt.Sprintf("%s: invalid JSON near byte %d", f.name, se.ByteOffset)}
		}
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return &Error{Code: CodeMalformedFile, Msg: f.name + ": the file ends early (truncated?)"}
		}
		var fe flate.CorruptInputError
		if errors.Is(err, zip.ErrChecksum) || errors.As(err, &fe) {
			return &Error{Code: CodeMalformedFile, Msg: f.name + ": corrupt data in the archive"}
		}
		return err
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return malformed(err)
	}
	if tok.Kind() != '[' {
		return &Error{Code: CodeMalformedFile, Msg: f.name + ": expected a JSON array of streaming history rows"}
	}
	for n := 0; dec.PeekKind() != ']'; n++ {
		val, err := dec.ReadValue()
		if err != nil {
			return malformed(err)
		}
		if val.Kind() != '{' {
			return &Error{Code: CodeMalformedFile, Msg: fmt.Sprintf("%s: row %d is not a JSON object", f.name, n+1)}
		}
		var e exportRow
		if err := json.Unmarshal(val, &e, unmarshalOpts); err != nil {
			return &Error{Code: CodeMalformedFile, Msg: fmt.Sprintf("%s: row %d has a field of the wrong type", f.name, n+1)}
		}
		r, err := extract(&e)
		if errors.Is(err, errBasicRow) {
			return errBasicExport
		}
		if err != nil {
			return &Error{Code: CodeMalformedFile, Msg: fmt.Sprintf("%s: row %d: %v", f.name, n+1, err)}
		}
		im.batch = append(im.batch, pending{row: r, video: f.video, json: string(val)})
		im.st.RowsDone++
		if len(im.batch) >= BatchSize {
			im.st.BytesDone = base + cnt.n
			if err := im.flush(ctx); err != nil {
				return err
			}
		}
	}
	if _, err := dec.ReadToken(); err != nil { // the closing bracket
		return malformed(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullBool(b *bool) any {
	if b == nil {
		return nil
	}
	if *b {
		return int64(1)
	}
	return int64(0)
}

// flush inserts the pending batch and records progress in one
// transaction.
func (im *importer) flush(ctx context.Context) error {
	if len(im.batch) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := im.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var inserted int64
	byType := map[string]int64{}
	for i := range im.batch {
		p := &im.batch[i]
		video := int64(0)
		if p.video {
			video = 1
		}
		res, err := stmt.ExecContext(ctx, im.jobID, p.hash[:], p.tsMs, p.msPlayed, p.contentType, nullStr(p.uri),
			nullStr(p.trackName), nullStr(p.artistName), nullStr(p.albumName), nullStr(p.reasonStart), nullStr(p.reasonEnd),
			nullBool(p.shuffle), nullBool(p.skipped), nullBool(p.offline), nullBool(p.incognito),
			nullStr(p.platform), nullStr(p.country), video, p.json)
		if err != nil {
			return fmt.Errorf("insert export row: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
			byType[p.contentType]++
		}
	}
	st := im.st
	st.RowsNew += inserted
	st.RowsDuplicate += int64(len(im.batch)) - inserted
	if _, err := tx.ExecContext(ctx, `UPDATE imports SET files_done = ?, bytes_done = ?, rows_done = ?, rows_new = ?,
		rows_duplicate = ? WHERE id = ?`, st.FilesDone, st.BytesDone, st.RowsDone, st.RowsNew, st.RowsDuplicate, im.jobID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	im.st.RowsNew, im.st.RowsDuplicate = st.RowsNew, st.RowsDuplicate
	for k, v := range byType {
		im.st.NewByType[k] += v
	}
	im.batch = im.batch[:0]
	if im.progress != nil {
		im.progress(im.st)
	}
	return nil
}

// started marks the job running with what the source revealed.
func (im *importer) started(ctx context.Context) error {
	var total any
	if im.st.BytesTotal > 0 {
		total = im.st.BytesTotal
	}
	_, err := im.db.Write.ExecContext(ctx, `UPDATE imports SET state = 'running', started_at = ?, files = ?, bytes_total = ?
		WHERE id = ?`, im.now().UnixMilli(), im.st.Files, total, im.jobID)
	return err
}

// finish records the outcome of job id.
func finish(ctx context.Context, d *db.DB, id int64, st Stats, runErr error, now time.Time) error {
	state, code, msg := "done", sql.NullString{}, sql.NullString{}
	var rowsTotal any = st.RowsDone
	if runErr != nil {
		state = "failed"
		c, m := errorCode(runErr)
		code, msg = sql.NullString{String: c, Valid: true}, sql.NullString{String: m, Valid: true}
		rowsTotal = nil
	}
	bytesDone := st.BytesDone
	if runErr == nil {
		bytesDone = st.BytesTotal
	}
	_, err := d.Write.ExecContext(ctx, `UPDATE imports SET state = ?, source_path = NULL, files = ?, files_done = ?,
		bytes_done = ?, rows_total = ?, rows_done = ?, rows_new = ?, rows_duplicate = ?, finished_at = ?,
		error_code = ?, error = ? WHERE id = ?`,
		state, st.Files, st.FilesDone, bytesDone, rowsTotal, st.RowsDone, st.RowsNew, st.RowsDuplicate,
		now.UnixMilli(), code, msg, id)
	return err
}

// execute runs job id over the file at path.
func execute(ctx context.Context, d *db.DB, id int64, path string, kind Kind, name string, now func() time.Time,
	progress func(Stats)) (Stats, error) {
	im := &importer{db: d, jobID: id, progress: progress, now: now, batch: make([]pending, 0, BatchSize)}
	src, err := openSource(path, kind, name)
	if err == nil {
		err = im.run(ctx, src)
		src.close()
	}
	// The outcome is recorded even when ctx was cancelled.
	if ferr := finish(context.WithoutCancel(ctx), d, id, im.st, err, now()); ferr != nil && err == nil {
		err = ferr
	}
	return im.st, err
}
