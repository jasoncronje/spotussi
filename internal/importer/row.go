package importer

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// exportRow is the subset of an Extended Streaming History row that is
// extracted into columns. Every field is optional in the parser: older
// exports lack the audiobook and offline fields, and nulls are common.
// The full row is stored verbatim regardless.
type exportRow struct {
	TS                 *string  `json:"ts"`
	MsPlayed           *float64 `json:"ms_played"` // float: tolerate 1234.0
	Platform           *string  `json:"platform"`
	ConnCountry        *string  `json:"conn_country"`
	TrackName          *string  `json:"master_metadata_track_name"`
	ArtistName         *string  `json:"master_metadata_album_artist_name"`
	AlbumName          *string  `json:"master_metadata_album_album_name"`
	TrackURI           *string  `json:"spotify_track_uri"`
	EpisodeName        *string  `json:"episode_name"`
	EpisodeURI         *string  `json:"spotify_episode_uri"`
	AudiobookTitle     *string  `json:"audiobook_title"`
	AudiobookURI       *string  `json:"audiobook_uri"`
	AudiobookChapterID *string  `json:"audiobook_chapter_uri"`
	ReasonStart        *string  `json:"reason_start"`
	ReasonEnd          *string  `json:"reason_end"`
	Shuffle            *bool    `json:"shuffle"`
	Skipped            *bool    `json:"skipped"`
	Offline            *bool    `json:"offline"`
	Incognito          *bool    `json:"incognito_mode"`

	// EndTime marks a basic Account Data export row
	// (StreamingHistory_music_*.json), which is out of scope (D10).
	EndTime *string `json:"endTime"`
}

// Content types of raw_export_rows.content_type.
const (
	ContentTrack     = "track"
	ContentEpisode   = "episode"
	ContentAudiobook = "audiobook"
	ContentUnknown   = "unknown"
)

// row is one parsed export row, ready to insert.
type row struct {
	hash        [sha256.Size]byte
	tsMs        int64
	msPlayed    int64
	contentType string
	uri         string
	trackName   string
	artistName  string
	albumName   string
	reasonStart string
	reasonEnd   string
	shuffle     *bool
	skipped     *bool
	offline     *bool
	incognito   *bool
	platform    string
	country     string
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// errRow describes why a row cannot be imported (without its contents:
// rows carry IP addresses and listening history, DF4).
type errRow string

func (e errRow) Error() string { return string(e) }

// extract fills the columns of r from e. ts and ms_played are required;
// everything else falls back to null.
func extract(e *exportRow) (row, error) {
	var r row
	if e.TS == nil {
		if e.EndTime != nil {
			return r, errBasicRow
		}
		return r, errRow("missing ts")
	}
	ts, err := time.Parse(time.RFC3339, *e.TS)
	if err != nil {
		return r, errRow("ts is not an RFC 3339 timestamp")
	}
	if e.MsPlayed == nil || *e.MsPlayed < 0 {
		return r, errRow("missing or negative ms_played")
	}
	r.tsMs, r.msPlayed = ts.UnixMilli(), int64(*e.MsPlayed)
	switch {
	case e.TrackURI != nil:
		r.contentType, r.uri = ContentTrack, *e.TrackURI
	case e.EpisodeURI != nil:
		r.contentType, r.uri = ContentEpisode, *e.EpisodeURI
	case e.AudiobookChapterID != nil:
		r.contentType, r.uri = ContentAudiobook, *e.AudiobookChapterID
	case e.AudiobookURI != nil || e.AudiobookTitle != nil:
		r.contentType = ContentAudiobook
	case e.EpisodeName != nil:
		r.contentType = ContentEpisode
	default:
		r.contentType = ContentUnknown
	}
	r.trackName, r.artistName, r.albumName = str(e.TrackName), str(e.ArtistName), str(e.AlbumName)
	r.reasonStart, r.reasonEnd = str(e.ReasonStart), str(e.ReasonEnd)
	r.shuffle, r.skipped, r.offline, r.incognito = e.Shuffle, e.Skipped, e.Offline, e.Incognito
	r.platform, r.country = str(e.Platform), str(e.ConnCountry)
	r.hash = rowHash(r.tsMs, r.uri, r.msPlayed, r.trackName, r.artistName, str(e.EpisodeName))
	return r, nil
}

// rowHash is the dedupe key (D23): sha256 over ts, uri and ms_played, plus
// the track, artist and episode names when the URI is null. It hashes
// parsed values rather than the raw bytes, so a later export that
// reformats fields or adds new ones still dedupes against the same
// listening. Fields are length-prefixed so no two inputs collide by
// concatenation.
func rowHash(tsMs int64, uri string, msPlayed int64, track, artist, episode string) [sha256.Size]byte {
	h := sha256.New()
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(tsMs))
	h.Write(n[:])
	field := func(s string) {
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	field(uri)
	binary.BigEndian.PutUint64(n[:], uint64(msPlayed))
	h.Write(n[:])
	if uri == "" {
		field(track)
		field(artist)
		field(episode)
	}
	var out [sha256.Size]byte
	h.Sum(out[:0])
	return out
}
