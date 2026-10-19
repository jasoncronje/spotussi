// Package capture is live capture (D2): the player poller, its state
// machine, and the recently-played backstop.
//
// The state machine (Machine) is pure and deterministic: it is driven only
// by player snapshots, each stamped with the local clock time it was
// observed at, and returns the plays that completed. All I/O (polling,
// scheduling, persistence) lives in Service.
//
// Measuring listening time: between two snapshots of the same play, the
// credited time is the observed progress delta, bounded by the wall-clock
// time between them. Playback position and listening time differ only
// when the owner seeks, so:
//
//   - progress advanced by about the elapsed time (or less, after a
//     pause): credit the progress delta;
//   - progress jumped (seek) while playing at both snapshots: credit the
//     elapsed wall-clock time, since listening was continuous, just at
//     another position;
//   - progress jumped while paused at either snapshot: credit nothing
//     (unknowable; the conservative choice);
//   - progress went back to (near) the start: a restart, i.e. repeat-one
//     or "previous", which closes the play and opens a new one.
//
// When a play ends (another track, an ad or episode, or nothing playing),
// the time since the last snapshot is split at the moment the next item
// started (its progress tells when), and credited to the old play only if
// it was playing, up to its remaining duration.
package capture

import (
	"encoding/json"
	"time"
)

// Kind is what the player is doing (currently_playing_type).
type Kind string

const (
	KindNothing Kind = "nothing" // 204 No Content or no device
	KindTrack   Kind = "track"
	KindEpisode Kind = "episode" // not requested (D11): a timeline gap
	KindAd      Kind = "ad"
	KindUnknown Kind = "unknown"
)

// Snapshot is one observation of the player.
type Snapshot struct {
	// At is the local clock time (epoch ms) the response arrived.
	At         int64
	Kind       Kind
	Playing    bool
	ProgressMs int64
	// Track is set for KindTrack.
	Track      *Track
	Shuffle    bool
	Repeat     string
	ContextURI string
	DeviceName string
	DeviceType string
	Private    bool
	// Raw is the verbatim /me/player response.
	Raw json.RawMessage
}

// Track identifies the playing track.
type Track struct {
	ID         string // Spotify ID, or the URI for local files
	URI        string
	DurationMs int64
}

// End reasons of a play.
const (
	EndTrackChange = "track_change" // another track started
	EndRestart     = "restart"      // the same track started over (repeat-one, previous)
	EndStopped     = "stopped"      // nothing is playing any more
	EndGap         = "gap"          // an ad, episode or unknown item took over
)

// Play is a completed play.
type Play struct {
	TrackID        string
	TrackURI       string
	DurationMs     int64
	StartedMs      int64
	EndedMs        int64
	MsPlayed       int64
	Skipped        bool
	EndReason      string
	Shuffle        bool
	Repeat         string
	ContextURI     string
	DeviceName     string
	DeviceType     string
	Private        bool
	Seeks          int
	Pauses         int
	DeviceSwitches int
	Observations   int
	FirstSeenMs    int64
	LastSeenMs     int64
	// Raw is the last player snapshot that saw the play.
	Raw json.RawMessage
}

// Config tunes the state machine. Zero fields take the defaults.
type Config struct {
	// ToleranceMs absorbs clock jitter and request latency when comparing
	// progress with elapsed time (plus 5% of the elapsed time).
	ToleranceMs int64
	// TrustGapMs is the longest gap between snapshots over which playback
	// is assumed continuous (for seeks and the first sighting).
	TrustGapMs int64
	// SkipMarginMs: a play replaced by another track that ended more than
	// this before its end is a skip. Covers crossfade (up to 12 s).
	SkipMarginMs int64
	// RestartMinMs: a jump back to the start counts as a restart (new
	// play) if it rewound at least this much, or if the track had reached
	// its end.
	RestartMinMs int64
}

// DefaultConfig holds the defaults.
var DefaultConfig = Config{ToleranceMs: 2500, TrustGapMs: 60_000, SkipMarginMs: 15_000, RestartMinMs: 10_000}

func (c Config) withDefaults() Config {
	d := DefaultConfig
	if c.ToleranceMs > 0 {
		d.ToleranceMs = c.ToleranceMs
	}
	if c.TrustGapMs > 0 {
		d.TrustGapMs = c.TrustGapMs
	}
	if c.SkipMarginMs > 0 {
		d.SkipMarginMs = c.SkipMarginMs
	}
	if c.RestartMinMs > 0 {
		d.RestartMinMs = c.RestartMinMs
	}
	return d
}

// Open is the play in progress.
type Open struct {
	TrackID        string           `json:"track_id"`
	TrackURI       string           `json:"track_uri"`
	DurationMs     int64            `json:"duration_ms"`
	StartedMs      int64            `json:"started_ms"`
	FirstSeenMs    int64            `json:"first_seen_ms"`
	LastSeenMs     int64            `json:"last_seen_ms"`
	LastProgressMs int64            `json:"last_progress_ms"`
	LastPlaying    bool             `json:"last_playing"`
	EverPlaying    bool             `json:"ever_playing"`
	MsPlayed       int64            `json:"ms_played"`
	Shuffle        bool             `json:"shuffle"`
	Repeat         string           `json:"repeat"`
	ContextURI     string           `json:"context_uri"`
	Private        bool             `json:"private"`
	Seeks          int              `json:"seeks"`
	Pauses         int              `json:"pauses"`
	DeviceSwitches int              `json:"device_switches"`
	Observations   int              `json:"observations"`
	DeviceName     string           `json:"device_name"` // current device
	DeviceType     string           `json:"device_type"`
	DeviceMs       map[string]int64 `json:"device_ms"` // credited ms per device (name \x00 type)
	Raw            json.RawMessage  `json:"raw"`
}

// Last summarises the previous snapshot of any kind.
type Last struct {
	At      int64 `json:"at"`
	Kind    Kind  `json:"kind"`
	Playing bool  `json:"playing"`
}

// MachineState is the machine's whole state; it is persisted after every
// snapshot so a restart resumes mid-track.
type MachineState struct {
	Open *Open `json:"open,omitempty"`
	Last *Last `json:"last,omitempty"`
}

// Machine turns snapshots into plays. It is not safe for concurrent use.
type Machine struct {
	cfg Config
	st  MachineState
}

// NewMachine builds a machine resuming from st (zero value: fresh).
func NewMachine(cfg Config, st MachineState) *Machine {
	return &Machine{cfg: cfg.withDefaults(), st: st}
}

// State returns the current state (for persistence).
func (m *Machine) State() MachineState { return m.st }

// Current is the open play (nil when none).
func (m *Machine) Current() *Open { return m.st.Open }

func clamp(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }

func deviceKey(name, typ string) string { return name + "\x00" + typ }

// Observe feeds one snapshot and returns the play it completed, if any
// (a slice for convenience: zero or one element). Snapshots must be fed
// in time order.
func (m *Machine) Observe(s Snapshot) []Play {
	var out []Play
	emit := func(p *Play) {
		if p != nil {
			out = append(out, *p)
		}
	}
	isTrack := s.Kind == KindTrack && s.Track != nil
	if o := m.st.Open; o != nil {
		switch {
		case isTrack && s.Track.ID == o.TrackID:
			if m.isRestart(o, s) {
				emit(m.close(o, s, s.At-s.ProgressMs, EndRestart))
				m.st.Open = m.start(s)
			} else {
				m.advance(o, s)
			}
		case isTrack:
			emit(m.close(o, s, s.At-s.ProgressMs, EndTrackChange))
			m.st.Open = m.start(s)
		case s.Kind == KindNothing:
			emit(m.close(o, s, s.At, EndStopped))
			m.st.Open = nil
		default: // ad, episode, unknown: a gap in the music timeline (D11)
			emit(m.close(o, s, s.At-s.ProgressMs, EndGap))
			m.st.Open = nil
		}
	} else if isTrack {
		m.st.Open = m.start(s)
	}
	m.st.Last = &Last{At: s.At, Kind: s.Kind, Playing: s.Playing}
	return out
}

// tolerance for comparing a progress delta with elapsed time dt.
func (m *Machine) tolerance(dt int64) int64 { return m.cfg.ToleranceMs + max(dt, 0)/20 }

// isRestart: the same track's progress went back to a point it could
// have reached since the last snapshot only by starting over, and either
// the old position was near the end (repeat-one) or far enough from the
// start (previous / replay).
func (m *Machine) isRestart(o *Open, s Snapshot) bool {
	dt := s.At - o.LastSeenMs
	tol := m.tolerance(dt)
	dp := s.ProgressMs - o.LastProgressMs
	if dp >= -tol || s.ProgressMs > dt+tol {
		return false
	}
	reachedEnd := o.LastPlaying && o.LastProgressMs+(dt-s.ProgressMs) >= o.DurationMs-tol
	return reachedEnd || o.LastProgressMs-s.ProgressMs >= m.cfg.RestartMinMs
}

// advance credits a snapshot of the same play.
func (m *Machine) advance(o *Open, s Snapshot) {
	dt := s.At - o.LastSeenMs
	dp := s.ProgressMs - o.LastProgressMs
	tol := m.tolerance(dt)
	var credit int64
	switch {
	case dp >= -tol && dp <= dt+tol:
		credit = clamp(dp, 0, max(dt, 0))
	case o.LastPlaying && s.Playing && dt <= m.cfg.TrustGapMs:
		credit = max(dt, 0) // seek during continuous playback
		o.Seeks++
	default:
		o.Seeks++ // seek while paused, or across a long gap: unknowable
	}
	if o.LastPlaying && !s.Playing {
		o.Pauses++
	}
	m.credit(o, credit)
	if s.DeviceName != o.DeviceName || s.DeviceType != o.DeviceType {
		o.DeviceSwitches++
		o.DeviceName, o.DeviceType = s.DeviceName, s.DeviceType
	}
	o.LastSeenMs, o.LastProgressMs, o.LastPlaying = s.At, s.ProgressMs, s.Playing
	o.EverPlaying = o.EverPlaying || s.Playing
	o.Private = o.Private || s.Private
	o.Observations++
	o.Raw = s.Raw
}

// credit adds ms to the open play, attributed to the device it was
// playing on at the last snapshot.
func (m *Machine) credit(o *Open, ms int64) {
	if ms <= 0 {
		return
	}
	o.MsPlayed += ms
	if o.DeviceMs == nil {
		o.DeviceMs = map[string]int64{}
	}
	o.DeviceMs[deviceKey(o.DeviceName, o.DeviceType)] += ms
}

// close ends the open play. boundary is the estimated time the next item
// started (s.At when unknown); the old play is credited the time from its
// last snapshot to the boundary if it was playing, up to its remaining
// duration.
func (m *Machine) close(o *Open, s Snapshot, boundary int64, reason string) *Play {
	remaining := max(0, o.DurationMs-o.LastProgressMs)
	var credit int64
	if o.LastPlaying {
		credit = clamp(boundary-o.LastSeenMs, 0, remaining)
	}
	m.credit(o, credit)
	if !o.EverPlaying && o.MsPlayed == 0 {
		return nil // only ever seen paused: not a listen
	}
	endPos := o.LastProgressMs + credit
	p := &Play{
		TrackID: o.TrackID, TrackURI: o.TrackURI, DurationMs: o.DurationMs,
		StartedMs: o.StartedMs, EndedMs: o.LastSeenMs + credit, MsPlayed: o.MsPlayed,
		Skipped:   (reason == EndTrackChange || reason == EndRestart) && endPos < o.DurationMs-m.cfg.SkipMarginMs,
		EndReason: reason, Shuffle: o.Shuffle, Repeat: o.Repeat, ContextURI: o.ContextURI, Private: o.Private,
		Seeks: o.Seeks, Pauses: o.Pauses, DeviceSwitches: o.DeviceSwitches, Observations: o.Observations,
		FirstSeenMs: o.FirstSeenMs, LastSeenMs: o.LastSeenMs, Raw: o.Raw,
		DeviceName: o.DeviceName, DeviceType: o.DeviceType,
	}
	// The device that played most of it (ties: the last one).
	var best int64 = -1
	for k, ms := range o.DeviceMs {
		if ms > best || (ms == best && k == deviceKey(o.DeviceName, o.DeviceType)) {
			best = ms
			p.DeviceName, p.DeviceType = splitDevice(k)
		}
	}
	return p
}

func splitDevice(k string) (string, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// start opens a play from its first snapshot. The progress already made
// is credited if it fits in the time since the previous snapshot
// (playback started since then); after a long gap or on the very first
// snapshot it is credited in full if playing (it most likely played),
// and not at all if paused (it may be left over from long ago).
func (m *Machine) start(s Snapshot) *Open {
	o := &Open{TrackID: s.Track.ID, TrackURI: s.Track.URI, DurationMs: s.Track.DurationMs,
		StartedMs: s.At - s.ProgressMs, FirstSeenMs: s.At, LastSeenMs: s.At, LastProgressMs: s.ProgressMs,
		LastPlaying: s.Playing, EverPlaying: s.Playing, Shuffle: s.Shuffle, Repeat: s.Repeat,
		ContextURI: s.ContextURI, Private: s.Private, Observations: 1,
		DeviceName: s.DeviceName, DeviceType: s.DeviceType, Raw: s.Raw}
	var credit int64
	if l := m.st.Last; l != nil && s.At-l.At <= m.cfg.TrustGapMs {
		credit = clamp(s.ProgressMs, 0, s.At-l.At)
	} else if s.Playing {
		credit = max(s.ProgressMs, 0)
	}
	m.credit(o, credit)
	return o
}

// Intervals are the poll intervals (D2).
type Intervals struct {
	Playing time.Duration // while playing
	Idle    time.Duration // paused, idle, nothing playing, episodes
	// EndSlack is added to the expected track end so the end-of-track
	// poll lands just after the next track started.
	EndSlack time.Duration
	Min      time.Duration
}

// DefaultIntervals: ~12 s while playing, 90 s otherwise.
var DefaultIntervals = Intervals{Playing: 12 * time.Second, Idle: 90 * time.Second, EndSlack: 1500 * time.Millisecond, Min: 2 * time.Second}

// NextPoll is how long to wait after snapshot s: the playing interval, or
// sooner to land just after the expected end of the track (duration -
// progress); the idle interval when paused or nothing is playing.
func NextPoll(s Snapshot, iv Intervals) time.Duration {
	switch {
	case s.Kind == KindTrack && s.Playing && s.Track != nil:
		remaining := time.Duration(s.Track.DurationMs-s.ProgressMs) * time.Millisecond
		return max(iv.Min, min(iv.Playing, remaining+iv.EndSlack))
	case (s.Kind == KindAd || s.Kind == KindUnknown) && s.Playing:
		return iv.Playing // music resumes soon
	default:
		return iv.Idle
	}
}
