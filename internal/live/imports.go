package live

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"spotussi/internal/api"
	"spotussi/internal/importer"
)

// CreateImport queues an uploaded export (D23).
func (b *Backend) CreateImport(ctx context.Context, up api.ImportUpload) (*api.ImportJob, error) {
	if b.imports == nil {
		return nil, api.ErrNotImplemented
	}
	j, err := b.imports.Submit(ctx, up.Path, up.Filename, importer.Kind(up.Kind))
	if err != nil {
		return nil, err
	}
	w := wireJob(*j)
	return &w, nil
}

// Imports lists the 50 most recent jobs.
func (b *Backend) Imports(ctx context.Context) (*api.ImportJobList, error) {
	jobs, err := importer.List(ctx, b.db, 50)
	if err != nil {
		return nil, err
	}
	return &api.ImportJobList{Items: wireJobs(jobs)}, nil
}

// Import reads one job.
func (b *Backend) Import(ctx context.Context, id string) (*api.ImportJob, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: no import %q", api.ErrNotFound, id)
	}
	j, err := importer.Get(ctx, b.db, n)
	if errors.Is(err, importer.ErrNotFound) {
		return nil, fmt.Errorf("%w: no import %q", api.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	w := wireJob(*j)
	return &w, nil
}

func wireJobs(jobs []importer.Job) []api.ImportJob {
	out := make([]api.ImportJob, len(jobs))
	for i, j := range jobs {
		out[i] = wireJob(j)
	}
	return out
}

// wireJob converts a job; started_at is the upload time while queued.
func wireJob(j importer.Job) api.ImportJob {
	w := api.ImportJob{ID: strconv.FormatInt(j.ID, 10), Filename: j.Filename, Origin: j.Origin, State: j.State,
		Files: j.Files, FilesDone: j.FilesDone, BytesTotal: j.BytesTotal, BytesDone: j.BytesDone, RowsTotal: j.RowsTotal,
		RowsDone: j.RowsDone, RowsNew: j.RowsNew, RowsDuplicate: j.RowsDuplicate, StartedAt: j.CreatedAt,
		FinishedAt: j.FinishedAt, ErrorCode: strPtr(j.ErrorCode), Error: strPtr(j.Error)}
	if j.StartedAt != nil {
		w.StartedAt = *j.StartedAt
	}
	return w
}
