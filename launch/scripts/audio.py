#!/usr/bin/env python3
"""Builds the launch film's audio and its timing table (draft 11).

    cd launch && uv run --python 3.12 --with numpy --with scipy scripts/audio.py

Reads   public/audio/vo.wav      the approved voice take (one continuous read, pauses cut in at slide changes)
        public/audio/bgm-v3.wav  the EDM loop (124 BPM, flat: no intro, no drop, no ending)
Writes  public/audio/vo-v2.wav   the same take; only the lengths of the silent pauses at the eight slide
                                 changes move (tens of ms), so every slide change can sit on a beat
        public/audio/bgm-v4.wav  the same loop re-cut to the film: sped up 2.4% (127 BPM), a filtered intro
                                 that opens when spotussi arrives, the track's own fill and lift moved under
                                 the end card, and a final hit that rings out (it had no ending)
        src/timing.json          word cues, slide-change frames and the beat grid, read by Motion.tsx

Nothing is generated here: no new voice, no new music, no sound effects from a library. Deterministic.
"""
import json
import math
from pathlib import Path

import numpy as np
from scipy import signal
from scipy.io import wavfile

ROOT = Path(__file__).resolve().parent.parent
AUDIO = ROOT / "public" / "audio"
FPS = 30
VO_OFFSET = 0.3  # the voice file starts this far into the film

# ── Voice ────────────────────────────────────────────────────────────────────────────────────
# Word onsets in vo.wav, in seconds, read off its energy envelope (whisper's word starts include the
# silence before a phrase, so they run up to half a second early).
WORDS = {
    "spotify": 0.15, "comes": 1.01, "once": 1.16,
    "brand": 2.59, "every": 5.15, "on": 6.15,
    "real": 7.63, "top": 8.04, "notJust": 9.09,
    "numberOne": 10.46, "twelve": 11.89, "since": 12.81,
    "session": 14.80, "playByPlay": 16.14,
    "watch": 17.535, "week": 18.77,
    "logs": 19.955, "asItHappens": 20.80, "and": 21.56, "imports": 21.75,
    "desktop": 23.855, "pocket": 24.85,
    "signOff": 26.04, "free": 27.05, "selfHost": 28.64, "end": 30.30,
}
# Every slide change: its name, the first word of the new slide, the natural gap before that word
# (end of the previous phrase to the onset) and how far that gap may change, in seconds. Most may
# move a few hundredths either way. Two are held open on purpose: the intro slide needs a clean
# break before the history slide, and the year needs room to flip by before "On your own server".
SLIDES = [
    ("intro", "brand", 0.42, (-0.06, 0.17)),
    ("history", "every", 0.22, (0.10, 0.36)),
    ("server", "on", 0.10, (0.80, 1.25)),
    ("top", "real", 0.42, (-0.06, 0.17)),
    ("artist", "numberOne", 0.40, (-0.06, 0.17)),
    ("sessions", "session", 0.50, (-0.06, 0.17)),
    ("taste", "watch", 0.455, (-0.06, 0.17)),
    ("live", "logs", 0.415, (-0.06, 0.17)),
    ("devices", "desktop", 0.425, (-0.06, 0.17)),
    ("end", "signOff", 0.41, (-0.06, 0.17)),
]
# Two of the cut-in pauses landed inside a word's decaying tail, leaving a faint remnant after the hole.
# Mute the remnant so the word simply ends: (from, to) in vo.wav seconds.
REMNANTS = [(7.301, 7.43), (25.717, 25.80)]

# ── Music ────────────────────────────────────────────────────────────────────────────────────
BEAT_NATIVE = 0.48391  # seconds per beat in bgm-v3.wav (124 BPM), from a fit over its 84 kicks
KICK0 = 0.0125  # the first kick's onset
SPEED = 128 / 125  # +2.4%: plain resampling, so no time-stretch artefacts (pitch rises 0.4 semitone)
BEAT = BEAT_NATIVE / SPEED
LEAD = (0.11, 0.25)  # a slide change lands on a beat this long before its first word
LIFT_BEAT = 32  # in the source: a two-beat fill, then hats and a crash come in on this beat
OUTRO_BEATS = 12  # three bars from the end card's downbeat to the final hit
TAIL = 1.05  # seconds the final hit rings before the film ends


def solve_pauses():
    """Choose the beat phase and the smallest pause changes that put every slide change on a beat.

    A dynamic programme over the slides. The state is the lead each cut has on its first word; the
    change to each pause then follows from two neighbouring leads, up to a whole number of beats.
    """
    leads = np.arange(LEAD[0], LEAD[1] + 1e-9, 0.01)
    on = [WORDS[word] for _, word, _, _ in SLIDES]
    # cost[i][a]: the least total change with slide i's lead at leads[a]; the first pause stays as it is.
    cost = [[0.02 * abs(lead - 0.15) for lead in leads]]
    back = [[(None, 0.0)] * len(leads)]
    for i in range(1, len(SLIDES)):
        dmin, dmax = SLIDES[i][3]
        want = min(max(0.0, dmin), dmax)  # the change we would make if beats did not matter
        row, brow = [], []
        for lead in leads:
            best = (math.inf, None, 0.0)
            for a, prev in enumerate(leads):
                base = (lead - prev) - (on[i] - on[i - 1])
                fits = [base + k * BEAT for k in range(-12, 13) if dmin <= base + k * BEAT <= dmax]
                if fits and math.isfinite(cost[i - 1][a]):
                    d = min(fits, key=lambda v: abs(v - want))
                    total = cost[i - 1][a] + abs(d - want) + 0.02 * abs(lead - 0.15)
                    if total < best[0]:
                        best = (total, a, d)
            row.append(best[0])
            brow.append((best[1], best[2]))
        cost.append(row)
        back.append(brow)
    a = int(np.argmin(cost[-1]))
    if not math.isfinite(cost[-1][a]):
        raise SystemExit("no beat phase fits the slide changes")
    deltas = []
    for i in range(len(SLIDES) - 1, 0, -1):
        prev, d = back[i][a]
        deltas.append(d)
        a = prev
    deltas.append(0.0)
    deltas.reverse()
    return float((on[0] - leads[a]) % BEAT), [round(float(d), 3) for d in deltas]


def build_voice(deltas):
    sr, x = wavfile.read(AUDIO / "vo.wav")
    assert x.ndim == 1 and x.dtype == np.int16
    for a, b in REMNANTS:
        x[int(a * sr) : int(b * sr)] = 0
    z = np.concatenate([[0], (x == 0).astype(np.int8), [0]])
    edges = np.diff(z)
    runs = [(a, b) for a, b in zip(np.where(edges == 1)[0], np.where(edges == -1)[0]) if b - a > sr * 0.02]
    # Where each pause is changed: the run of digital silence already cut in before the word, or,
    # where there is none, the quietest moment of the natural gap (faded 4 ms either side).
    edits = []
    for (_, word, gap, _), d in zip(SLIDES, deltas):
        on = WORDS[word]
        before = [r for r in runs if r[1] / sr <= on and on - r[1] / sr < 0.5]
        if before:
            edits.append((before[-1][0], before[-1][1], d))
            continue
        assert d >= 0, word
        lo, hi = int((on - gap) * sr), int((on - 0.03) * sr)
        win = int(0.01 * sr)
        level = [float(np.mean(x[i : i + win].astype(np.float64) ** 2)) for i in range(lo, hi - win, win // 4)]
        at = lo + int(np.argmin(level)) * (win // 4) + win // 2
        n = int(0.004 * sr)
        x[at - n : at] = (x[at - n : at] * np.linspace(1, 0, n)).astype(np.int16)
        x[at : at + n] = (x[at : at + n] * np.linspace(0, 1, n)).astype(np.int16)
        edits.append((at, at, d))
    out, pos = [], 0
    for a, b, d in edits:
        n = (b - a) + int(round(d * sr))
        assert n >= 0 and a >= pos
        out += [x[pos:a], np.zeros(n, np.int16)]
        pos = b
    out.append(x[pos:])
    y = np.concatenate(out)
    wavfile.write(AUDIO / "vo-v2.wav", sr, y)
    starts = [a / sr for a, _, _ in edits]
    return lambda t: t + sum(d for s, d in zip(starts, deltas) if s < t), len(y) / sr


def svf_lowpass_sweep(x, sr, cutoff):
    """Two cascaded 2-pole state-variable low-passes with a per-sample cutoff (12 dB/oct each)."""
    g = np.tan(np.pi * np.minimum(cutoff, sr * 0.45) / sr)
    k = 1 / 0.707
    a1 = 1 / (1 + g * (g + k))
    a2 = g * a1
    a3 = g * a2
    y = x.copy()
    for ch in range(y.shape[1]):
        for _ in range(2):
            ic1 = ic2 = 0.0
            col = y[:, ch].tolist()
            for i, v in enumerate(col):
                v3 = v - ic2
                v1 = a1[i] * ic1 + a2[i] * v3
                v2 = ic2 + a2[i] * ic1 + a3[i] * v3
                ic1 = 2 * v1 - ic1
                ic2 = 2 * v2 - ic2
                col[i] = v2
            y[:, ch] = col
    return y


def crash(sr, seconds, decay, level, seed):
    """A noise crash, the kind techno uses on a downbeat: band-passed noise with a fast attack."""
    n = int(seconds * sr)
    noise = np.random.default_rng(seed).standard_normal((n, 2))
    noise = signal.sosfilt(signal.butter(2, (4500, 13000), "bandpass", fs=sr, output="sos"), noise, axis=0)
    t = np.arange(n) / sr
    env = np.minimum(1, t / 0.003) * np.exp(-t / decay)
    return noise / np.abs(noise).max() * env[:, None] * level


def build_music(phase, drop_beat, intro_open, duration):
    sr, raw = wavfile.read(AUDIO / "bgm-v3.wav")
    y = raw.astype(np.float64) / 32768
    kick = lambda n: KICK0 + BEAT_NATIVE * n
    pre, xf = 0.010, 96  # splice 10 ms before a kick, with a 2 ms crossfade
    cut = lambda n: kick(n) - pre

    # The source runs 8 bars, then a fill into the lift on beat 32. The film needs ~56 beats before
    # its end card, so play the opening four bars again first (bars 1 and 5 are the closest match in
    # the track, so bar 4 → bar 1 is the join the track itself makes), entering part-way through.
    lead_in = (drop_beat + phase / BEAT) * BEAT_NATIVE  # native seconds before the lift
    rem = lead_in - (kick(LIFT_BEAT) - cut(0))
    blocks = [(cut(0), kick(LIFT_BEAT + OUTRO_BEATS) + 1.0)]
    while rem > 16 * BEAT_NATIVE:
        blocks.insert(0, (cut(0), cut(16)))
        rem -= 16 * BEAT_NATIVE
    blocks.insert(0, (cut(16) - rem, cut(16)))
    seq = y[int(round(blocks[0][0] * sr)) : int(round(blocks[0][1] * sr))]
    ramp = np.linspace(0, 1, xf)[:, None]
    for a, b in blocks[1:]:
        nxt = y[int(round(a * sr)) - xf : int(round(b * sr))]
        seq[-xf:] = seq[-xf:] * (1 - ramp) + nxt[:xf] * ramp
        seq = np.concatenate([seq, nxt[xf:]])
    seq = signal.resample_poly(seq, 125, 128, axis=0)

    n = int(round(duration * sr))
    seq = seq[:n] if len(seq) >= n else np.concatenate([seq, np.zeros((n - len(seq), 2))])
    t = np.arange(n) / sr
    t_drop = phase + drop_beat * BEAT
    t_final = t_drop + OUTRO_BEATS * BEAT

    # Intro: the loop is muffled, as if from the next room, and opens over the two beats before
    # spotussi arrives. One move, on the film's first slide change.
    m = int((intro_open + 0.02) * sr)
    sweep = np.clip((t[:m] - (intro_open - 2 * BEAT)) / (2 * BEAT), 0, 1)
    wet = svf_lowpass_sweep(seq[:m], sr, 400 * (18000 / 400) ** sweep)
    g = np.clip((t[:m] - intro_open) / 0.02, 0, 1)[:, None]
    seq[:m] = wet * (1 - g) + seq[:m] * g
    seq *= np.clip(t / 0.3, 0, 1)[:, None]

    # Before the end card: the track's own two-beat fill, with the low end thinned so the downbeat lands.
    a, b = int((t_drop - 2 * BEAT) * sr), int(t_drop * sr)
    thin = signal.sosfilt(signal.butter(2, 220, "high", fs=sr, output="sos"), seq[a - sr // 2 : b], axis=0)[sr // 2 :]
    tt = t[a:b]
    g = (np.clip((tt - tt[0]) / (2 * BEAT), 0, 1) * np.clip((t_drop - 0.003 - tt) / 0.006, 0, 1))[:, None]
    seq[a:b] = seq[a:b] * (1 - g) + thin * g

    # The ending the track never had: the final downbeat hits, the loop stops and the hit rings out.
    i = int(t_final * sr)
    send = seq[i - 200 : i + int(0.26 * sr)].copy()
    send *= (np.clip(np.arange(len(send)) / 200, 0, 1) * np.clip((len(send) - np.arange(len(send))) / (0.04 * sr), 0, 1))[:, None]
    seq *= np.where(t < t_final + 0.12, 1.0, np.exp(-(t - t_final - 0.12) / 0.06))[:, None]
    ir_t = np.arange(int(2.4 * sr)) / sr
    ir = np.random.default_rng(11).standard_normal((len(ir_t), 2)) * np.exp(-ir_t * 6.91 / 2.0)[:, None]
    ir = signal.sosfilt(signal.butter(2, (200, 6000), "bandpass", fs=sr, output="sos"), ir, axis=0)
    ir = np.concatenate([np.zeros((int(0.02 * sr), 2)), ir / np.sqrt((ir**2).sum(axis=0))])
    tail = np.stack([signal.fftconvolve(send[:, ch], ir[:, ch]) for ch in range(2)], axis=1) * 0.9
    end = min(n, i - 200 + len(tail))
    seq[i - 200 : end] += tail[: end - (i - 200)]

    for at, c in ((t_drop, crash(sr, 1.6, 0.32, 0.05, 7)), (t_final, crash(sr, 1.4, 0.5, 0.07, 9))):
        j = int(at * sr)
        k = min(n, j + len(c))
        seq[j:k] += c[: k - j]

    seq *= (0.5 - 0.5 * np.cos(np.pi * np.clip((duration - t) / 0.2, 0, 1)))[:, None]
    peak = np.abs(seq).max()
    if peak > 0.97:
        seq *= 0.97 / peak
    wavfile.write(AUDIO / "bgm-v4.wav", sr, np.round(seq * 32767).astype(np.int16))
    return t_final


def main():
    c, deltas = solve_pauses()
    remap, vo_len = build_voice(deltas)
    words = {k: round(remap(v) + VO_OFFSET, 3) for k, v in WORDS.items()}  # film seconds
    phase = (c + VO_OFFSET) % BEAT
    beat_of = lambda w: math.floor((words[w] - LEAD[0] + 0.015 - phase) / BEAT)  # 15 ms of slack for rounding
    cut_beats = {name: beat_of(word) for name, word, _, _ in SLIDES}
    t_final = phase + (cut_beats["end"] + OUTRO_BEATS) * BEAT
    frames = int(round((t_final + TAIL) * FPS))
    build_music(phase, cut_beats["end"], phase + cut_beats["intro"] * BEAT, frames / FPS)
    timing = {
        "fps": FPS,
        "duration": frames,
        "voOffset": VO_OFFSET,
        "beat": {"phase": round(phase, 4), "period": round(BEAT, 6)},
        "cutBeats": cut_beats,
        "finalBeat": cut_beats["end"] + OUTRO_BEATS,
        "words": words,
    }
    (ROOT / "src" / "timing.json").write_text(json.dumps(timing, indent=2) + "\n")
    print("pause changes (ms):", [int(d * 1000) for d in deltas], "net", round(sum(deltas), 3))
    print("beat phase %.4f period %.5f (%.1f BPM)" % (phase, BEAT, 60 / BEAT))
    for name, w, _, _ in SLIDES:
        tb = phase + cut_beats[name] * BEAT
        print("  %-9s beat %2d  f%5.1f  first word f%5.1f  lead %.1f f" % (name, cut_beats[name], tb * FPS, words[w] * FPS, (words[w] - tb) * FPS))
    print("voice ends %.2f s, final hit %.2f s (f%.1f), film %d frames = %.2f s, vo-v2 %.2f s" % (words["end"], t_final, t_final * FPS, frames, frames / FPS, vo_len))


if __name__ == "__main__":
    main()
