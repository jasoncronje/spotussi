package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Database download (D15, D28). GET /system/backup writes the same
// snapshot as `spotussi backup` to a temporary file next to the database
// and streams it. API tokens are refused in requireAuth: the file holds
// the password hash, token hashes and the encrypted Spotify refresh token.

// backupSource is implemented by *db.DB.
type backupSource interface {
	Backup(ctx context.Context, dst string) error
	Dir() string
}

// backupTimeout replaces the server's write timeout for a download.
const backupTimeout = 2 * time.Hour

func registerBackup(mux *http.ServeMux, src backupSource) {
	// Downloads interrupted by a crash leave their temp files behind.
	for _, pat := range []string{".download-*.db", "..download-*.db.partial-*"} {
		stale, _ := filepath.Glob(filepath.Join(src.Dir(), pat))
		for _, p := range stale {
			os.Remove(p)
		}
	}
	var running sync.Mutex
	mux.HandleFunc("GET "+v1+"/system/backup", func(w http.ResponseWriter, r *http.Request) {
		if !running.TryLock() {
			writeError(w, r, http.StatusConflict, "backup_running", "another backup is being written; try again when it finishes")
			return
		}
		defer running.Unlock()
		var rnd [8]byte
		rand.Read(rnd[:])
		tmp := filepath.Join(src.Dir(), ".download-"+hex.EncodeToString(rnd[:])+".db")
		defer os.Remove(tmp)
		start := time.Now()
		if err := src.Backup(r.Context(), tmp); err != nil {
			if r.Context().Err() == nil {
				slog.Error("backup download", "err", err)
			}
			writeError(w, r, http.StatusInternalServerError, "backup_failed", "the snapshot could not be written; see the server log")
			return
		}
		f, err := os.Open(tmp)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		slog.Info("backup download", "bytes", fi.Size(), "snapshot_took", time.Since(start).Round(time.Millisecond))
		// Not supported by every ResponseWriter (tests); the server's own
		// timeout then applies.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(backupTimeout))
		h := w.Header()
		h.Set("Content-Type", "application/vnd.sqlite3")
		h.Set("Content-Disposition", `attachment; filename="`+BackupFilename(start)+`"`)
		h.Set("Cache-Control", "no-store")
		http.ServeContent(w, r, "", fi.ModTime(), f)
	})
}

// BackupFilename names a snapshot taken at t: spotussi-YYYYMMDD-HHMMSS.db
// (UTC), as both the download and the CLI default suggest.
func BackupFilename(t time.Time) string {
	return "spotussi-" + t.UTC().Format("20060102-150405") + ".db"
}
