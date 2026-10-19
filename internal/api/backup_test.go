package api

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"spotussi/internal/auth"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
)

func TestBackupDownload(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	// A leftover from a crashed download is cleaned up at startup.
	stale := filepath.Join(dir, ".download-0011223344556677.db")
	os.WriteFile(stale, []byte("x"), 0o600)
	keys, _ := keyring.New(bytes.Repeat([]byte{3}, 32))
	a := auth.New(auth.Options{DB: d, Keyring: keys, Mode: config.AuthPassword,
		Hash: auth.Params{MemoryKiB: 64, Time: 1, Threads: 1}})
	s := &server{t: t, auth: a, h: NewHandler(Options{Version: "t", Mode: "live", DB: d, Auth: a})}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale download file kept: %v", err)
	}

	expect(t, s.do(req{method: "GET", path: "/api/v1/system/backup"}), 401, "unauthorized")
	cookie, _, _ := s.setup(req{})
	rec := s.do(req{method: "GET", path: "/api/v1/system/backup", cookie: cookie})
	expect(t, rec, 200, "")
	h := rec.Header()
	if h.Get("Content-Type") != "application/vnd.sqlite3" || h.Get("Cache-Control") != "no-store" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), `attachment; filename="spotussi-`) {
		t.Fatalf("headers: %v", h)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatalf("body is not a SQLite file: %q", rec.Body.Bytes()[:min(32, rec.Body.Len())])
	}
	// The download is the database, owner password included, and passes
	// the integrity check.
	out := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(out, rec.Body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(ctx, out); err != nil {
		t.Fatal(err)
	}
	r, err := db.OpenPath(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if setup, err := auth.New(auth.Options{DB: r, Mode: config.AuthPassword}).SetupRequired(ctx); err != nil || setup {
		t.Fatalf("restored copy: setup required %v, %v", setup, err)
	}
	// Nothing is left next to the database.
	left, _ := filepath.Glob(filepath.Join(dir, "*download*"))
	if len(left) > 0 {
		t.Fatalf("temp files left: %v", left)
	}
}
