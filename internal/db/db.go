// Package db owns the SQLite database: connections, pragmas and migrations.
//
// SQLite runs in WAL mode (D4) through the pure-Go modernc.org/sqlite driver
// (D5, no cgo). Two pools are opened on the same file: a single-connection
// writer, so writes queue in Go instead of contending for SQLite's write
// lock, and a small reader pool that WAL lets run concurrently with it.
//
// Schema rule (DF1): timestamps are integer epoch milliseconds and columns use
// plain INTEGER/TEXT/REAL types, so DuckDB can attach the file later.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// FileName is the database file inside the data directory.
const FileName = "spotussi.db"

// DB holds the writer and reader pools for one database file.
type DB struct {
	Write *sql.DB
	Read  *sql.DB
	Path  string
	// lock is the shared lock that keeps a restore from replacing the
	// file underneath this process (see lockFile).
	lock *os.File
}

// ErrInUse means another process holds the database in a way that
// conflicts: a restore while it is open, or opening it during a restore.
var ErrInUse = errors.New("database in use")

func closeLock(f *os.File) {
	if f != nil {
		f.Close() // releases the flock
	}
}

// pragmas are applied to every new connection by the driver.
var pragmas = []string{
	"journal_mode(WAL)",
	"busy_timeout(5000)",
	"foreign_keys(ON)",
	"synchronous(NORMAL)", // safe with WAL: a crash can lose the last commits, never corrupt
	"temp_store(MEMORY)",
	"cache_size(-2000)", // ~2 MB page cache per connection
}

// idleTime closes connections idle that long, which frees their page
// caches: the server spends most of its time idle, and its memory budget
// is small (D24).
const idleTime = 30 * time.Second

// Open opens (creating if needed) the database in dataDir and applies all
// pending migrations.
func Open(ctx context.Context, dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return OpenPath(ctx, filepath.Join(dataDir, FileName))
}

// OpenPath opens the database at an explicit file path and migrates it.
func OpenPath(ctx context.Context, path string) (*DB, error) {
	lock, err := lockFile(path, false)
	if errors.Is(err, ErrInUse) {
		return nil, fmt.Errorf("%s: a restore is running", path)
	} else if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	write, err := openPool(path, true)
	if err != nil {
		closeLock(lock)
		return nil, err
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	write.SetConnMaxLifetime(0)
	write.SetConnMaxIdleTime(idleTime)

	// Migrate before opening readers so they never see an old schema.
	if err := Migrate(ctx, write); err != nil {
		write.Close()
		closeLock(lock)
		return nil, err
	}

	read, err := openPool(path, false)
	if err != nil {
		write.Close()
		closeLock(lock)
		return nil, err
	}
	n := max(2, min(runtime.NumCPU(), 8))
	read.SetMaxOpenConns(n)
	read.SetMaxIdleConns(n)
	read.SetConnMaxIdleTime(idleTime)

	return &DB{Write: write, Read: read, Path: path, lock: lock}, nil
}

func openPool(path string, writer bool) (*sql.DB, error) {
	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	if writer {
		// BEGIN IMMEDIATE takes the write lock up front, avoiding
		// SQLITE_BUSY upgrades in the middle of a transaction.
		q.Set("_txlock", "immediate")
	} else {
		q.Add("_pragma", "query_only(ON)")
	}
	dsn := "file:" + path + "?" + q.Encode()
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return pool, nil
}

// Dir is the directory holding the database file.
func (d *DB) Dir() string { return filepath.Dir(d.Path) }

// Ping checks both pools.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.Write.PingContext(ctx); err != nil {
		return err
	}
	return d.Read.PingContext(ctx)
}

// Close closes both pools. The writer closes last so the final WAL
// checkpoint happens after readers are gone.
func (d *DB) Close() error {
	rerr := d.Read.Close()
	werr := d.Write.Close()
	closeLock(d.lock)
	if werr != nil {
		return werr
	}
	return rerr
}
