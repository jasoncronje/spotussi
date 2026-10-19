package api

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"unicode"

	"spotussi/internal/importer"
)

// Export uploads (D23). POST /imports takes the file itself as the body
// (no multipart), streams it to a temp file under the import directory
// and hands it to the backend, which queues the import and answers 202.
// Unsafe-method rules (owner session plus CSRF; API tokens are
// read-only) come from requireAuth like every other route.

// uploadTimeout replaces the server's read and write timeouts for an
// upload: a 2 GiB export over a slow link takes far longer than 30 s.
const uploadTimeout = 2 * time.Hour

func registerImports(mux *http.ServeMux, b Backend, dir string, maxBytes int64) {
	get(mux, v1+"/imports", func(r *http.Request, _ *params) (*ImportJobList, error) {
		return b.Imports(r.Context())
	})
	get(mux, v1+"/imports/{id}", func(r *http.Request, _ *params) (*ImportJob, error) {
		return b.Import(r.Context(), r.PathValue("id"))
	})
	mux.HandleFunc("POST "+v1+"/imports", func(w http.ResponseWriter, r *http.Request) {
		job, err := upload(w, r, b, dir, maxBytes)
		if err != nil {
			writeResult[ImportJob](w, r, nil, err)
			return
		}
		writeJSON(w, r, http.StatusAccepted, job)
	})
}

// zipTypes and jsonTypes are the Content-Types trusted when the first
// bytes are inconclusive.
var (
	zipTypes  = map[string]bool{"application/zip": true, "application/x-zip-compressed": true, "application/x-zip": true}
	jsonTypes = map[string]bool{"application/json": true, "text/json": true}
)

func upload(w http.ResponseWriter, r *http.Request, b Backend, dir string, maxBytes int64) (*ImportJob, error) {
	if dir == "" {
		return nil, ErrNotImplemented
	}
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(uploadTimeout)
	// Not supported by every ResponseWriter (tests); the server's own
	// timeouts then apply.
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
	if r.ContentLength > maxBytes {
		return nil, tooLarge(maxBytes)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, importer.UploadPrefix+"*")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	n, err := io.Copy(f, http.MaxBytesReader(w, r.Body, maxBytes))
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &mbe):
		return nil, tooLarge(maxBytes)
	case err != nil:
		if r.Context().Err() == nil {
			slog.Warn("import upload failed", "err", err, "bytes", n)
		}
		return nil, errorf(http.StatusBadRequest, "upload_failed", "the upload did not complete")
	case n == 0:
		return nil, errorf(http.StatusBadRequest, "bad_request", "the request body is empty; send the export file as the body")
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	kind, err := uploadKind(f.Name(), r.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	name := uploadName(r, kind)
	job, err := b.CreateImport(r.Context(), ImportUpload{Path: f.Name(), Filename: name, Kind: string(kind), Size: n})
	if err != nil {
		return nil, err
	}
	keep = true
	return job, nil
}

func tooLarge(maxBytes int64) error {
	return errorf(http.StatusRequestEntityTooLarge, "too_large",
		"the file is larger than this instance accepts (%d MiB; IMPORT_MAX_BYTES)", maxBytes>>20)
}

// uploadKind sniffs the stored upload, falling back to Content-Type.
func uploadKind(p, contentType string) (importer.Kind, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	f.Close()
	if k, ok := importer.Sniff(head[:n]); ok {
		return k, nil
	}
	mt, _, _ := mime.ParseMediaType(contentType)
	switch {
	case zipTypes[mt]:
		return importer.KindZip, nil
	case jsonTypes[mt]:
		return importer.KindJSON, nil
	}
	return "", errorf(http.StatusUnsupportedMediaType, "unsupported_file",
		"not a zip archive or JSON file; upload my_spotify_data.zip or a Streaming_History_Audio_*.json file")
}

// uploadName is the display name: the filename parameter, else the
// Content-Disposition filename, else a default for the kind.
func uploadName(r *http.Request, kind importer.Kind) string {
	name := r.URL.Query().Get("filename")
	if name == "" {
		if _, params, err := mime.ParseMediaType(r.Header.Get("Content-Disposition")); err == nil {
			name = params["filename"]
		}
	}
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, name)
	if len(name) > 255 {
		name = strings.ToValidUTF8(name[:255], "")
	}
	if name == "" || name == "." || name == "/" {
		name = "upload." + string(kind)
	}
	return name
}
