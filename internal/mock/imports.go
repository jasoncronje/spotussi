package mock

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"spotussi/internal/api"
	"spotussi/internal/refdata"
)

// Simulated imports, so the upload UI can be built against mock mode.
// Uploads are accepted like on a real instance (the file is detected and
// stored by the HTTP layer), then discarded: the mock reports a job that
// is queued for a moment, runs for a few seconds with advancing progress
// fields, and finishes. Nothing is added to the dataset. A file named
// like the basic export (StreamingHistory*.json) fails with basic_export.

const (
	mockQueuedMs  = 1_000
	mockRunningMs = 8_000
	mockRowBytes  = 700 // roughly one pretty-printed export row
)

type mockImport struct {
	id       int
	filename string
	size     int64
	created  int64
	basic    bool
}

type mockImports struct {
	mu   sync.Mutex
	jobs []mockImport // oldest first
}

// seedImport is the synthetic dataset's own export import.
func (b *Backend) seedImport() api.ImportJob {
	ds := b.ds
	var rows int64
	for _, p := range ds.Plays {
		if p.Has(refdata.FlagSrcExport) {
			rows++
		}
	}
	start := ds.ExportUntil + 3*86_400_000
	return api.ImportJob{ID: "1", Filename: "my_spotify_data.zip", Origin: "upload", State: "done", Files: 15, FilesDone: 15,
		BytesTotal: ptr(rows * mockRowBytes), BytesDone: rows * mockRowBytes, RowsTotal: ptr(rows), RowsDone: rows,
		RowsNew: rows, StartedAt: start, FinishedAt: ptr(start + 41_000)}
}

// job reports j as of now.
func (j mockImport) job(now int64) api.ImportJob {
	rows := max(1, j.size/mockRowBytes)
	out := api.ImportJob{ID: strconv.Itoa(j.id), Filename: j.filename, Origin: "upload", State: "queued", StartedAt: j.created}
	elapsed := now - j.created
	if elapsed < mockQueuedMs {
		return out
	}
	out.StartedAt = j.created + mockQueuedMs
	out.Files = 1 + int(j.size/(12<<20))
	out.BytesTotal = ptr(j.size)
	if j.basic {
		out.State, out.FinishedAt = "failed", ptr(out.StartedAt)
		out.ErrorCode = ptr("basic_export")
		out.Error = ptr("this is the basic Account Data export (StreamingHistory_music_*.json), which has no track IDs; " +
			"request the Extended Streaming History export from Spotify's privacy page and upload that")
		return out
	}
	frac := min(1, float64(elapsed-mockQueuedMs)/mockRunningMs)
	out.State = "running"
	out.FilesDone = int(frac * float64(out.Files))
	out.BytesDone = int64(frac * float64(j.size))
	out.RowsDone = int64(frac * float64(rows))
	// A re-upload of an export the mock already "has": most rows are
	// already imported.
	out.RowsDuplicate = out.RowsDone * 9 / 10
	out.RowsNew = out.RowsDone - out.RowsDuplicate
	if frac == 1 {
		out.State, out.FilesDone, out.RowsTotal = "done", out.Files, ptr(rows)
		out.FinishedAt = ptr(out.StartedAt + mockRunningMs)
	}
	return out
}

// CreateImport accepts an upload and simulates its import.
func (b *Backend) CreateImport(ctx context.Context, up api.ImportUpload) (*api.ImportJob, error) {
	os.Remove(up.Path) // the mock keeps nothing
	b.imports.mu.Lock()
	defer b.imports.mu.Unlock()
	base := path.Base(up.Filename)
	j := mockImport{id: len(b.imports.jobs) + 2, filename: up.Filename, size: up.Size, created: b.clock().UnixMilli(),
		basic: strings.HasPrefix(base, "StreamingHistory") && strings.HasSuffix(base, ".json")}
	b.imports.jobs = append(b.imports.jobs, j)
	out := j.job(j.created)
	return &out, nil
}

// importList returns up to limit jobs, newest first.
func (b *Backend) importList(limit int) []api.ImportJob {
	b.imports.mu.Lock()
	defer b.imports.mu.Unlock()
	now := b.clock().UnixMilli()
	out := []api.ImportJob{}
	for i := len(b.imports.jobs) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, b.imports.jobs[i].job(now))
	}
	if len(out) < limit {
		out = append(out, b.seedImport())
	}
	return out
}

func (b *Backend) Imports(ctx context.Context) (*api.ImportJobList, error) {
	return &api.ImportJobList{Items: b.importList(50)}, nil
}

func (b *Backend) Import(ctx context.Context, id string) (*api.ImportJob, error) {
	for _, j := range b.importList(1 << 30) {
		if j.ID == id {
			return &j, nil
		}
	}
	return nil, fmt.Errorf("%w: no import %q", api.ErrNotFound, id)
}
