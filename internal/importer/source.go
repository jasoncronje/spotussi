package importer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Kind is what an import reads.
type Kind string

const (
	KindZip  Kind = "zip"  // the export package
	KindJSON Kind = "json" // one history file
	KindDir  Kind = "dir"  // an unpacked package (CLI only)
)

var (
	// historyFile matches the Extended Streaming History files at any
	// depth (D10, D23). Video files hold video podcast rows (DF7).
	historyFile = regexp.MustCompile(`^Streaming_History_(Audio|Video)_.*\.json$`)
	// basicFile matches the basic Account Data package's history
	// (StreamingHistory_music_0.json, older StreamingHistory0.json),
	// which is out of scope (D10).
	basicFile = regexp.MustCompile(`^StreamingHistory.*\.json$`)
)

// file is one history file inside a source.
type file struct {
	name  string // path inside the source, for messages
	size  int64  // uncompressed bytes (-1: unknown)
	video bool
	open  func() (io.ReadCloser, error)
}

// source is the set of history files an import reads.
type source struct {
	files []file
	close func() error
}

// Sniff detects a file's kind from its first bytes: a zip local file (or
// empty archive) header, or JSON (an array or object after optional BOM
// and whitespace). ok is false when neither matches.
func Sniff(head []byte) (k Kind, ok bool) {
	if bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")) {
		return KindZip, true
	}
	head = bytes.TrimLeft(bytes.TrimPrefix(head, []byte("\xef\xbb\xbf")), " \t\r\n")
	if len(head) > 0 && (head[0] == '[' || head[0] == '{') {
		return KindJSON, true
	}
	return "", false
}

// SniffPath detects the kind of the file or directory at p.
func SniffPath(p string) (Kind, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return KindDir, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if k, ok := Sniff(head[:n]); ok {
		return k, nil
	}
	return "", &Error{Code: CodeUnsupportedFile, Msg: "not a zip archive or JSON file"}
}

// openSource lists the history files at p.
func openSource(p string, kind Kind, displayName string) (*source, error) {
	switch kind {
	case KindZip:
		return openZip(p)
	case KindDir:
		return openDir(p)
	case KindJSON:
		base := path.Base(filepath.ToSlash(displayName))
		if basicFile.MatchString(base) {
			return nil, errBasicExport
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		f := file{name: base, size: st.Size(), video: strings.HasPrefix(base, "Streaming_History_Video_"),
			open: func() (io.ReadCloser, error) { return os.Open(p) }}
		return &source{files: []file{f}, close: func() error { return nil }}, nil
	}
	return nil, fmt.Errorf("unknown import kind %q", kind)
}

// pick keeps the history files among names (slash paths) and reports
// whether the basic export's history was seen instead.
func pick(name string) (keep, video, basic bool) {
	base := path.Base(name)
	if strings.HasPrefix(base, "._") { // macOS resource forks
		return false, false, false
	}
	if m := historyFile.FindStringSubmatch(base); m != nil {
		return true, m[1] == "Video", false
	}
	return false, false, basicFile.MatchString(base)
}

func noFiles(basic bool) error {
	if basic {
		return errBasicExport
	}
	return &Error{Code: CodeNoHistoryFiles, Msg: "no Streaming_History_Audio_*.json or Streaming_History_Video_*.json files found; " +
		"upload the Extended Streaming History export (my_spotify_data.zip)"}
}

func openZip(p string) (*source, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, &Error{Code: CodeMalformedFile, Msg: "not a readable zip archive"}
	}
	src := &source{close: zr.Close}
	basic := false
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		keep, video, b := pick(zf.Name)
		basic = basic || b
		if !keep {
			continue
		}
		src.files = append(src.files, file{name: zf.Name, size: int64(zf.UncompressedSize64), video: video,
			open: zf.Open})
	}
	if len(src.files) == 0 {
		zr.Close()
		return nil, noFiles(basic)
	}
	sortFiles(src.files)
	return src, nil
}

func openDir(dir string) (*source, error) {
	src := &source{close: func() error { return nil }}
	basic := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		keep, video, b := pick(filepath.ToSlash(rel))
		basic = basic || b
		if !keep {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		src.files = append(src.files, file{name: filepath.ToSlash(rel), size: info.Size(), video: video,
			open: func() (io.ReadCloser, error) { return os.Open(p) }})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(src.files) == 0 {
		return nil, noFiles(basic)
	}
	sortFiles(src.files)
	return src, nil
}

func sortFiles(fs []file) {
	slices.SortFunc(fs, func(a, b file) int { return strings.Compare(a.name, b.name) })
}

// bomSkipper drops a leading UTF-8 byte order mark, which the JSON
// decoder would reject.
func bomSkipper(r io.Reader) io.Reader {
	br := bufio.NewReaderSize(r, 64<<10)
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte("\xef\xbb\xbf")) {
		br.Discard(3)
	}
	return br
}

// counter counts bytes read through it.
type counter struct {
	r io.Reader
	n int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

var errBasicExport = &Error{Code: CodeBasicExport, Msg: "this is the basic Account Data export (StreamingHistory_music_*.json), " +
	"which has no track IDs; request the Extended Streaming History export from Spotify's privacy page and upload that"}

// errBasicRow marks a row in the basic export's format.
var errBasicRow = errors.New("basic export row")
