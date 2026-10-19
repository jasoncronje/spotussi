package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"spotussi/internal/query"
)

// gzipMinBytes is the smallest JSON body worth compressing.
const gzipMinBytes = 1024

var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return w
}}

// ETag returns a weak entity tag for body. Weak, because the same entity
// may be sent gzip-encoded or not (RFC 9110 §8.8.1).
func ETag(body []byte) string {
	h := fnv.New64a()
	h.Write(body)
	return `W/"` + strconv.FormatUint(h.Sum64(), 36) + `"`
}

// etagMatches implements If-None-Match comparison (weak comparison).
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for part := range strings.SplitSeq(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(part), "W/") == want {
			return true
		}
	}
	return false
}

// writeJSON sends v as JSON with an ETag, honouring If-None-Match for
// safe methods and gzip-compressing larger bodies.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("encode response", "path", r.URL.Path, "err", err)
		http.Error(w, `{"error":{"code":"internal","message":"encode failed"}}`, http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-cache")
	h.Add("Vary", "Accept-Encoding")
	if status == http.StatusOK && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		etag := ETag(body)
		h.Set("ETag", etag)
		if etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	if len(body) >= gzipMinBytes && acceptsEncoding(r, "gzip") {
		var buf bytes.Buffer
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(&buf)
		gz.Write(body)
		gz.Close()
		gzipPool.Put(gz)
		h.Set("Content-Encoding", "gzip")
		body = buf.Bytes()
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}

func acceptsEncoding(r *http.Request, enc string) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), enc) {
			continue
		}
		if k, v, ok := strings.Cut(strings.TrimSpace(params), "="); ok && strings.TrimSpace(k) == "q" {
			q, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			return err == nil && q > 0
		}
		return true
	}
	return false
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	writeJSON(w, r, status, ErrorBody{Error: ErrorDetail{Code: code, Message: msg}})
}

// writeResult maps a backend result or error to a response.
func writeResult[T any](w http.ResponseWriter, r *http.Request, v *T, err error) {
	var he *httpError
	switch {
	case err == nil:
		writeJSON(w, r, http.StatusOK, v)
	case errors.As(err, &he):
		if he.retryAfter > 0 {
			writeRetryAfter(w, he.retryAfter)
		}
		writeError(w, r, he.status, he.code, he.msg)
	case errors.Is(err, ErrNotImplemented):
		writeError(w, r, http.StatusNotImplemented, "not_implemented", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrBadRequest), errors.Is(err, query.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, r.Context().Err()):
		// Client went away; nothing useful to send.
	default:
		slog.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, r, http.StatusInternalServerError, "internal", "internal error")
	}
}
