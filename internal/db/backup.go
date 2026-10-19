package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Backups (D15, D28). A snapshot is written with VACUUM INTO, which reads
// the database inside one read transaction: with WAL, writers carry on
// while it runs and the copy is consistent as of the transaction's start.
// The copy is compacted, includes the WAL's committed pages and is a
// single self-contained file (rollback journal; the server switches it
// back to WAL when it opens it).
//
// The instance key (DATA_DIR/key) is not part of the snapshot.

// ErrBackupExists is returned when the destination file exists.
var ErrBackupExists = errors.New("backup destination already exists")

// Backup writes a consistent snapshot of the database to dst and checks
// it (see BackupFile).
func (d *DB) Backup(ctx context.Context, dst string) error {
	return BackupFile(ctx, d.Path, dst)
}

// BackupFile snapshots the SQLite database at src into dst while other
// processes may be writing to it. dst must not exist. The snapshot is
// written to a temporary file next to dst, checked with PRAGMA
// integrity_check, and renamed into place, so dst only ever appears
// complete. It is created with mode 0600: it holds the password hash, API
// token hashes and the encrypted Spotify refresh token.
func BackupFile(ctx context.Context, src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup source: %w", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%w: %s", ErrBackupExists, dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".partial-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// VACUUM INTO accepts an existing empty file, which keeps the mode
	// CreateTemp gave it (0600).
	tmp.Close()
	done := false
	defer func() {
		if !done {
			os.Remove(tmpPath)
		}
	}()
	if err := vacuumInto(ctx, src, tmpPath); err != nil {
		return err
	}
	if err := Verify(ctx, tmpPath); err != nil {
		return err
	}
	if err := syncFile(tmpPath); err != nil {
		return err
	}
	// Link instead of rename so an existing dst created meanwhile is
	// never replaced.
	if err := os.Link(tmpPath, dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrBackupExists, dst)
		}
		if err := os.Rename(tmpPath, dst); err != nil { // filesystems without hard links
			return err
		}
	}
	done = true
	os.Remove(tmpPath)
	return nil
}

// vacuumInto runs VACUUM INTO on a connection of its own, so neither of
// the server's pools is held while the copy is written.
func vacuumInto(ctx context.Context, src, dst string) error {
	pool, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	if _, err := pool.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return fmt.Errorf("snapshot %s: %w", src, err)
	}
	return nil
}

// Verify opens the database file at path read-only and runs PRAGMA
// integrity_check on it.
func Verify(ctx context.Context, path string) error {
	pool, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer pool.Close()
	rows, err := pool.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("integrity check %s: %w", path, err)
	}
	defer rows.Close()
	var problems []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		if s != "ok" {
			problems = append(problems, s)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("integrity check %s: %w", path, err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("integrity check %s failed: %s", path, strings.Join(problems, "; "))
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Restore replaces the database at dst with the snapshot at src. It
// refuses while any process has dst open (ErrInUse: stop the server
// first), checks src with PRAGMA integrity_check, copies it next to dst,
// keeps the current database as dst.before-restore-<unix ms> (with its WAL
// checkpointed into it), removes the old -wal and -shm files, which
// SQLite would otherwise replay onto the restored file, and renames the
// copy into place. It returns the kept file's path ("" if dst did not
// exist). Migrations run when the server next opens it.
func Restore(ctx context.Context, src, dst string, now func() int64) (kept string, err error) {
	if err := Verify(ctx, src); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	lock, err := lockFile(dst, true)
	if errors.Is(err, ErrInUse) {
		return "", fmt.Errorf("%w: %s is open in another process; stop the server first", ErrInUse, dst)
	} else if err != nil {
		return "", fmt.Errorf("lock %s: %w", dst, err)
	}
	defer closeLock(lock)

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".restore-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	in, err := os.Open(src)
	if err != nil {
		tmp.Close()
		return "", err
	}
	_, err = io.Copy(tmp, in)
	in.Close()
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("copy %s: %w", src, err)
	}

	if _, err := os.Stat(dst); err == nil {
		// Fold the old WAL into the old file before keeping it, so the
		// kept copy is complete on its own.
		if err := checkpoint(ctx, dst); err != nil {
			return "", err
		}
		kept = fmt.Sprintf("%s.before-restore-%d", dst, now())
		if err := os.Rename(dst, kept); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(dst + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return kept, err
		}
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return kept, err
	}
	return kept, nil
}

func checkpoint(ctx context.Context, path string) error {
	pool, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := pool.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint %s: %w", path, err)
	}
	return nil
}
