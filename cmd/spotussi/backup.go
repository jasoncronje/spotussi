package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
)

// backupCmd writes a consistent snapshot of the database in DATA_DIR to
// the given file, or into the given directory (created if the argument
// ends in a slash) as
// spotussi-YYYYMMDD-HHMMSS.db (D28). It is safe while the server runs.
// The database is not migrated or otherwise written to.
func backupCmd(args []string) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New("usage: spotussi backup <file | directory/>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	src := filepath.Join(cfg.DataDir, db.FileName)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("no database at %s (set DATA_DIR): %w", src, err)
	}
	dst := args[0]
	start := time.Now()
	// A trailing slash names a directory, created if needed (the image
	// has no shell to mkdir with).
	if strings.HasSuffix(dst, "/") {
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return err
		}
	}
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		dst = filepath.Join(dst, api.BackupFilename(start))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := db.BackupFile(ctx, src, dst); err != nil {
		return err
	}
	fi, err := os.Stat(dst)
	if err != nil {
		return err
	}
	fmt.Printf("Backed up %s to %s (%.1f MB, integrity check ok) in %s\n", src, dst,
		float64(fi.Size())/1e6, time.Since(start).Round(10*time.Millisecond))
	fmt.Printf("The instance key %s is not in the backup: keep a copy of it too (it decrypts the Spotify connection).\n",
		filepath.Join(cfg.DataDir, "key"))
	return nil
}

// restoreCmd replaces the database in DATA_DIR with a backup, with the
// server stopped (db.Restore refuses otherwise). --key also installs the
// instance key that belongs to the backup (needed on another machine to
// keep the Spotify connection); the replaced database and key are kept
// next to them as *.before-restore-<unix ms>.
func restoreCmd(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	keyPath := fs.String("key", "", "instance key file to install with the backup (the old DATA_DIR/key is kept)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: spotussi restore [--key <key file>] <backup.db>   (stop the server first)")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var key []byte
	if *keyPath != "" {
		if key, err = os.ReadFile(*keyPath); err != nil {
			return err
		}
		if len(key) != 32 {
			return fmt.Errorf("%s is not an instance key (want 32 bytes, got %d)", *keyPath, len(key))
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	now := time.Now().UnixMilli()
	dst := filepath.Join(cfg.DataDir, db.FileName)
	kept, err := db.Restore(ctx, fs.Arg(0), dst, func() int64 { return now })
	if err != nil {
		return err
	}
	// The lock file too: on a fresh volume this run created it.
	if err := chownLikeDir(cfg.DataDir, dst, dst+".lock"); err != nil {
		return err
	}
	fmt.Printf("Restored %s from %s (integrity check ok).\n", dst, fs.Arg(0))
	if kept != "" {
		fmt.Printf("The previous database is kept as %s; delete it once the restore looks right.\n", kept)
	}
	if key != nil {
		kp := filepath.Join(cfg.DataDir, keyring.FileName)
		if _, err := os.Stat(kp); err == nil {
			old := fmt.Sprintf("%s.before-restore-%d", kp, now)
			if err := os.Rename(kp, old); err != nil {
				return err
			}
			fmt.Printf("The previous key is kept as %s.\n", old)
		}
		if err := os.WriteFile(kp, key, 0o600); err != nil {
			return err
		}
		if err := chownLikeDir(cfg.DataDir, kp); err != nil {
			return err
		}
		fmt.Printf("Installed the instance key from %s.\n", *keyPath)
	} else {
		fmt.Println("Without the backup's instance key (--key) the Spotify connection can't be decrypted: reconnect Spotify in Settings after starting.")
	}
	return nil
}
