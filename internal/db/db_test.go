package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndSetsPragmas(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var mode string
	if err := d.Read.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	for pragma, want := range map[string]int{"foreign_keys": 1, "synchronous": 1, "busy_timeout": 5000} {
		var got int
		if err := d.Write.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", pragma, got, want)
		}
	}

	if _, err := d.Write.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES ('timezone', '"UTC"', 1)`); err != nil {
		t.Fatal(err)
	}
	var v string
	if err := d.Read.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'timezone'`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != `"UTC"` {
		t.Errorf("value = %q", v)
	}

	// Readers are query-only.
	if _, err := d.Read.ExecContext(ctx, `DELETE FROM settings`); err == nil {
		t.Error("expected read pool to reject writes")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	for range 2 {
		d, err := OpenPath(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
	}
	d, err := OpenPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ms, _ := loadMigrations()
	var n int
	if err := d.Read.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(ms) {
		t.Errorf("schema_migrations rows = %d, want %d", n, len(ms))
	}
}

// A migration numbered below one already applied (branches merged out of
// order) is still applied.
func TestAppliesSkippedVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	d, err := OpenPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := loadMigrations()
	first := ms[0]
	if _, err := d.Write.Exec(`DELETE FROM schema_migrations WHERE version = ?`, first.version); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Write.Exec(`DROP TABLE settings`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = OpenPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SetSetting(ctx, "k", "v"); err != nil {
		t.Fatalf("settings table not recreated: %v", err)
	}
}

// An instance that ran the ListenBrainz build (schema at 0009, deployed
// before library sync was numbered) upgrades to 0010 alone: 0009 is not
// run again (its ADD COLUMN would fail) and the library tables appear.
func TestUpgradesFrom0009ListenBrainz(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	d, err := OpenPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`DELETE FROM schema_migrations WHERE version >= 10`,
		`DROP TABLE library_spans`, `DROP TABLE library_walks`,
		`DROP TABLE library_playlists`, `DROP TABLE library_syncs`,
	} {
		if _, err := d.Write.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	d.Close()
	d, err = OpenPath(ctx, path)
	if err != nil {
		t.Fatalf("upgrade from 0009: %v", err)
	}
	defer d.Close()
	var names []string
	rs, err := d.Read.Query(`SELECT name FROM schema_migrations WHERE version IN (9, 10) ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	for rs.Next() {
		var n string
		if err := rs.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	rs.Close()
	if len(names) != 2 || names[0] != "0009_listenbrainz.sql" || names[1] != "0010_library.sql" {
		t.Fatalf("migrations 9 and 10 = %v", names)
	}
	if _, err := d.Read.Exec(`SELECT COUNT(*) FROM library_syncs`); err != nil {
		t.Errorf("library tables not created: %v", err)
	}
}

func TestRefusesNewerSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	d, err := OpenPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Write.Exec(`INSERT INTO schema_migrations VALUES (9999, 'future', 0)`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if _, err := OpenPath(ctx, path); err == nil {
		t.Fatal("expected error for newer schema")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var s string
	if ok, err := d.GetSetting(ctx, "k", &s); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
	for _, v := range []string{"a", "b"} {
		if err := d.SetSetting(ctx, "k", v); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := d.GetSetting(ctx, "k", &s); !ok || err != nil || s != "b" {
		t.Fatalf("got %q %v %v", s, ok, err)
	}
}
