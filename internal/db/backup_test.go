package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBackupWhileWriting(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Write.ExecContext(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY, a INTEGER NOT NULL, b INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	insert := func(n int) {
		tx, err := d.Write.BeginTx(ctx, nil)
		if err != nil {
			t.Error(err)
			return
		}
		for i := range n {
			// Two rows per transaction: a torn snapshot would see an
			// odd count.
			if _, err := tx.ExecContext(ctx, `INSERT INTO t (a, b) VALUES (?, ?), (?, ?)`, i, -i, i, -i); err != nil {
				t.Error(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Error(err)
		}
	}
	insert(5000)

	// Keep writing (one statement per commit) while snapshots are taken.
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() {
		for !stop.Load() {
			if _, err := d.Write.ExecContext(ctx, `INSERT INTO t (a, b) VALUES (1, -1), (2, -2)`); err != nil {
				t.Error(err)
				return
			}
		}
	})
	var counts []int
	for i := range 3 {
		dst := filepath.Join(dir, fmt.Sprintf("snap-%d.db", i))
		if err := d.Backup(ctx, dst); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("snapshot mode %v, want 0600", fi.Mode().Perm())
		}
		snap, err := OpenPath(ctx, dst)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := snap.Read.QueryRowContext(ctx, `SELECT count(*) FROM t`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		snap.Close()
		if n%2 != 0 || n < 10000 {
			t.Errorf("snapshot %d has %d rows", i, n)
		}
		counts = append(counts, n)
	}
	stop.Store(true)
	wg.Wait()
	t.Logf("snapshot row counts: %v", counts)

	// No partial files are left behind; an existing destination is refused.
	matches, _ := filepath.Glob(filepath.Join(dir, ".*partial*"))
	if len(matches) > 0 {
		t.Errorf("leftover temp files: %v", matches)
	}
	if err := d.Backup(ctx, filepath.Join(dir, "snap-0.db")); !errors.Is(err, ErrBackupExists) {
		t.Errorf("backup over an existing file: %v", err)
	}
	if err := BackupFile(ctx, filepath.Join(dir, "missing.db"), filepath.Join(dir, "x.db")); err == nil {
		t.Error("backup of a missing database succeeded")
	}
}

func TestVerifyRejectsCorruptFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "snap.db")
	if err := d.Backup(ctx, dst); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if err := Verify(ctx, dst); err != nil {
		t.Fatal(err)
	}
	// Overwrite the middle of the file with garbage.
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	for i := 4096; i < len(b) && i < 4096*4; i++ {
		b[i] = 0xA5
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, dst); err == nil {
		t.Error("corrupt file passed verification")
	}
	if err := os.WriteFile(dst, []byte("not a database at all, just text that is long enough to be a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, dst); err == nil {
		t.Error("text file passed verification")
	}
}

func TestRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := d.Write.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE t (v TEXT)`)
	exec(`INSERT INTO t VALUES ('in backup')`)
	snap := filepath.Join(t.TempDir(), "snap.db")
	if err := d.Backup(ctx, snap); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO t VALUES ('after backup')`) // lives in the WAL until checkpointed

	now := func() int64 { return 1234 }
	if _, err := Restore(ctx, snap, d.Path, now); !errors.Is(err, ErrInUse) {
		t.Fatalf("restore while open: %v", err)
	}
	d.Close()
	if _, err := Restore(ctx, filepath.Join(dir, "missing.db"), d.Path, now); err == nil {
		t.Fatal("restore from a missing file succeeded")
	}
	kept, err := Restore(ctx, snap, d.Path, now)
	if err != nil {
		t.Fatal(err)
	}
	if kept != d.Path+".before-restore-1234" {
		t.Fatalf("kept %q", kept)
	}
	count := func(path string) int {
		t.Helper()
		x, err := OpenPath(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer x.Close()
		var n int
		if err := x.Read.QueryRowContext(ctx, `SELECT count(*) FROM t`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(d.Path); n != 1 {
		t.Errorf("restored database has %d rows, want 1", n)
	}
	if n := count(kept); n != 2 {
		t.Errorf("kept database has %d rows, want 2 (WAL folded in)", n)
	}
	// Opening a restored database takes the shared lock again; a restore
	// then refuses, and an open during a restore would too.
	x, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if _, err := Restore(ctx, snap, x.Path, now); !errors.Is(err, ErrInUse) {
		t.Fatalf("second restore while open: %v", err)
	}
}
