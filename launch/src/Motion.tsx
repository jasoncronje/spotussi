// The spotussi launch film (draft 18), built as motion design. Every piece of UI is drawn in code in
// the app's own design language (graphite panels, Archivo, lamp meters), with invented artists and
// generated cover art. Voice-over and music only. Ends on the call to action.
//
// The order: the hook (Wrapped comes once a year), the introduction (what spotussi is, said once),
// the answer to the hook (not once a year: every single day, your whole history, on your own
// server), five features, the three ways in (web app,
// iOS app, your AI agent over MCP), the sign-off.
//
// 35 s at 30 fps. All timing comes from timing.json, which scripts/audio.py writes next to the
// audio it builds: `words` are the frames at which the voice says each cue word, and every slide
// change sits on a beat of the music, a few frames before the slide's first word.
import type React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, interpolateColors, staticFile, useCurrentFrame } from "remotion";
import { C, FONT, mulberry32 } from "./kit";
import { LAMP_ORDER, Phone, headStyle } from "./parts";
import TIMING from "./timing.json";

const FPS = 30;
export const DURATION = TIMING.duration;
/** The frame at which the voice says this cue word. */
const w = (k: keyof typeof TIMING.words) => Math.round(TIMING.words[k] * FPS);
/** The frame of the music's nth beat. */
const beat = (n: number) => Math.round((TIMING.beat.phase + n * TIMING.beat.period) * FPS);
/** Slide changes, each on a beat. */
const cutAt = (k: keyof typeof TIMING.cutBeats) => beat(TIMING.cutBeats[k]);
const CUT = {
  intro: cutAt("intro"),
  history: cutAt("history"),
  server: w("on") - 5, // not a slide change: the server rises under the pad as she says "on your own server"
  top: cutAt("top"),
  artist: cutAt("artist"),
  sessions: cutAt("sessions"),
  taste: cutAt("taste"),
  live: cutAt("live"),
  devices: cutAt("devices"),
  end: cutAt("end"),
};
/** The music's final hit; the film fades to black as it rings out. */
const FINAL = beat(TIMING.finalBeat);
const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
const E = {
  out: Easing.bezier(0.16, 1, 0.3, 1),
  soft: Easing.bezier(0.22, 1, 0.36, 1),
  inOut: Easing.bezier(0.65, 0, 0.35, 1),
  in: Easing.bezier(0.7, 0, 0.84, 0),
  land: Easing.bezier(0.34, 1.2, 0.5, 1),
  emph: Easing.bezier(0.2, 0, 0, 1), // for things that travel: quick away, long settle
};
const r = (f: number, a: number, d: number, e: (t: number) => number = E.out) => e(Math.min(1, Math.max(0, (f - a) / Math.max(1, d))));
const mix = (a: number, b: number, t: number) => a + (b - a) * t;
const fmt = (n: number) => Math.round(n).toLocaleString("en-US");

const PANEL = "#15161a";
const EDGE = "#2a2c33";
const MUTE = "#8b8f99";
const DIM = "#5b5f68";

// ── Generated cover art: seeded gradients and shapes. Invented artists, no real artwork.
const PALETTES = [
  ["#ff6b4a", "#ffb347", "#3a0d0d"],
  ["#7b5cff", "#ff6ec7", "#140a2e"],
  ["#00c2a8", "#0a5cff", "#03132b"],
  ["#f5d547", "#ff7a00", "#2b1500"],
  ["#e9e9ee", "#7a7d88", "#16171b"],
  ["#ff4d6d", "#5a189a", "#12001f"],
  ["#2ec4b6", "#cbf3f0", "#05211f"],
  ["#9bf6ff", "#4361ee", "#070b2b"],
  ["#c3f73a", "#1db954", "#06200f"],
  ["#ffd6a5", "#ff8fab", "#2a0e14"],
];
const Art = ({ seed, size, round = false, style }: { seed: number; size: number; round?: boolean; style?: React.CSSProperties }) => {
  const rnd = mulberry32(seed * 977 + 13);
  const [a, b, d] = PALETTES[seed % PALETTES.length];
  const kind = seed % 4;
  const id = `g${seed}`;
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" style={{ display: "block", borderRadius: round ? "50%" : size * 0.08, flex: "none", ...style }}>
      <defs>
        <linearGradient id={id} x1={rnd()} y1="0" x2={1 - rnd()} y2="1">
          <stop offset="0" stopColor={a} />
          <stop offset="1" stopColor={b} />
        </linearGradient>
      </defs>
      <rect width="100" height="100" fill={d} />
      {kind === 0 ? <circle cx={30 + rnd() * 40} cy={30 + rnd() * 40} r={34} fill={`url(#${id})`} /> : null}
      {kind === 1 ? [0, 1, 2, 3, 4].map((i) => <rect key={i} x="0" y={i * 22 - 4} width="100" height="12" fill={`url(#${id})`} opacity={1 - i * 0.16} transform={`rotate(${-20 + rnd() * 10} 50 50)`} />) : null}
      {kind === 2 ? [40, 30, 20, 10].map((rr, i) => <circle key={rr} cx="50" cy="50" r={rr} fill="none" stroke={`url(#${id})`} strokeWidth="5" opacity={1 - i * 0.18} />) : null}
      {kind === 3 ? (
        <>
          <rect width="100" height="100" fill={`url(#${id})`} />
          <path d={`M0 ${60 + rnd() * 20} Q50 ${20 + rnd() * 30} 100 ${55 + rnd() * 25} V100 H0Z`} fill={d} opacity="0.85" />
        </>
      ) : null}
    </svg>
  );
};

// ── Shared pieces.
const Label = ({ children, style }: { children: React.ReactNode; style?: React.CSSProperties }) => (
  <div style={{ fontFamily: FONT, fontSize: 18, fontWeight: 560, color: MUTE, letterSpacing: "0.01em", ...style }}>{children}</div>
);

type Wd = { t: string; at: number; lit?: boolean };
/** A line of copy appears as one piece when its first word is spoken: a short fade and a small
 * rise, nothing per word. `\n` starts a new line, which appears on its own first word. A line whose
 * first word is `at: 0` is simply there when its slide fades in. `out` clears it. */
const Say = ({ words, f, size, out, style, align = "left" }: { words: Wd[]; f: number; size: number; out?: number; style?: React.CSSProperties; align?: "left" | "center" }) => {
  const gone = out !== undefined ? r(f, out, 6, E.in) : 0;
  if (gone >= 1) return null;
  const lines: Wd[][] = [[]];
  for (const w of words) (w.t === "\n" ? lines.push([]) : lines[lines.length - 1].push(w));
  return (
    <div style={{ position: "absolute", ...headStyle(size), opacity: 1 - gone, textAlign: align, ...style }}>
      {lines.map((ln, i) => {
        if (!ln.length) return null;
        const p = r(f, ln[0].at - 1, 10);
        return (
          <div key={i} style={{ opacity: p, transform: `translateY(${((1 - p) * 12).toFixed(1)}px)` }}>
            {ln.map((w, k) => (
              <span key={k} style={{ color: w.lit ? C.lamp : undefined }}>
                {k ? " " : ""}
                {w.t}
              </span>
            ))}
          </div>
        );
      })}
    </div>
  );
};

// ── The tear-off calendar's data.
const YEARS = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024, 2025, 2026];
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const MDAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
const WEEKDAYS = ["Thursday", "Friday", "Saturday", "Sunday", "Monday", "Tuesday", "Wednesday"]; // 2026 starts on a Thursday
const monthDay = (d: number) => {
  let m = 0;
  while (m < 11 && d >= MDAYS[m]) d -= MDAYS[m++];
  return { m: MONTHS[m], d: d + 1 };
};
const DAYS = (() => {
  const rnd = mulberry32(21);
  return Array.from({ length: 365 }, (_, i) => {
    const week = 0.55 + 0.45 * Math.sin((i / 7) * Math.PI * 2 + 1);
    const season = 0.65 + 0.35 * Math.sin((i / 365) * Math.PI * 2 - 1.2);
    return Math.max(0.08, Math.min(1, week * season * (0.55 + rnd() * 0.75)));
  });
})();
const SONGS = [
  ["Glass Hours", "Nova Reyes"], ["Night Signal", "Kairo"], ["Paper Moons", "Static Bloom"], ["Low Tide", "June Avenue"],
  ["Afterglow FM", "Lumen Drift"], ["Neon Harbor", "The Midnight Arcade"], ["Coastline", "Halcyon Coast"], ["Velvet Static", "Mira Vale"],
  ["Orbiting", "Paper Satellites"], ["Polar Lights", "Oslo Nights"],
];
const WRAPPED_DAY = 334; // Dec 1

type PageKind = "blank" | "full" | "wrapped";
const CalPage = ({ day, kind }: { day: number; kind: PageKind }) => {
  const { m, d } = monthDay(day);
  const wrapped = kind === "wrapped";
  const lit = kind !== "blank";
  const song = SONGS[(day * 7) % SONGS.length];
  const mins = Math.round(20 + DAYS[day] * 240);
  return (
    <div style={{ position: "absolute", inset: 0, borderRadius: 26, background: wrapped ? "#0f2a19" : "#17181c", boxShadow: `inset 0 0 0 1.5px ${wrapped ? "#1f6b3c" : EDGE}`, overflow: "hidden" }}>
      {/* Header band: month and weekday, like a desk calendar's red strip. A blank day's is graphite.
          The Wrapped page's is lit green, and so is every full page's: with spotussi, every day is
          the day you get your stats. */}
      <div style={{ height: 96, background: lit ? C.lamp : "#23252b", display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 36px" }}>
        <span style={{ ...headStyle(40), color: lit ? "#06140b" : C.paper }}>{m}</span>
        <span style={{ fontFamily: FONT, fontSize: 24, fontWeight: 600, color: lit ? "#06140b" : MUTE }}>{WEEKDAYS[day % 7]}</span>
      </div>
      <div style={{ position: "absolute", left: 36, top: 112, ...headStyle(230), color: wrapped ? C.lamp : C.paper, letterSpacing: "-0.05em" }}>{d}</div>
      <div style={{ position: "absolute", left: 36, right: 36, bottom: 40, height: 210 }}>
        {kind === "blank" ? (
          <div style={{ position: "absolute", inset: 0, borderRadius: 18, background: "#121316" }} />
        ) : null}
        {wrapped ? (
          <div style={{ position: "absolute", inset: 0 }}>
            <div style={{ ...headStyle(64), color: C.paper }}>Your Wrapped</div>
            <div style={{ fontFamily: FONT, fontSize: 28, color: "#9fd9b4", marginTop: 14 }}>Your year in stats, once a year.</div>
          </div>
        ) : null}
        {kind === "full" ? (
          <div style={{ position: "absolute", inset: 0, display: "flex", gap: 26, alignItems: "flex-start" }}>
            <Art seed={(day * 7) % 40} size={130} />
            <div style={{ flex: 1 }}>
              <div style={{ fontFamily: FONT, fontSize: 20, color: MUTE }}>Top song</div>
              <div style={{ fontFamily: FONT, fontSize: 32, fontWeight: 680, color: C.paper, marginTop: 4, whiteSpace: "nowrap" }}>{song[0]}</div>
              <div style={{ fontFamily: FONT, fontSize: 22, color: MUTE }}>{song[1]}</div>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginTop: 18 }}>
                <span style={{ ...headStyle(40), fontVariantNumeric: "tabular-nums" }}>{Math.floor(mins / 60)}h {String(mins % 60).padStart(2, "0")}m</span>
                <span style={{ fontFamily: FONT, fontSize: 20, color: MUTE }}>listened</span>
              </div>
              <div style={{ display: "flex", gap: 6, marginTop: 12 }}>
                {Array.from({ length: 16 }, (_, k) => <span key={k} style={{ width: 12, height: 12, borderRadius: 6, background: k < Math.round(DAYS[day] * 16) ? C.lamp : C.grille }} />)}
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
};

/** A flip schedule: page k starts flipping at times[k] and takes durs[k] frames. */
const schedule = (start: number, end: number, n: number, accel: number) => {
  const times: number[] = [];
  for (let k = 0; k < n; k++) times.push(start + (end - start) * Math.pow(k / n, accel));
  const durs = times.map((t, k) => Math.max(1.2, Math.min(10, ((k + 1 < n ? times[k + 1] : end) - t) * 1.6)));
  return { times, durs };
};
const CAL = { x: 1180, y: 190, w: 620, h: 720 };

/** The pad: its page edges, the page under the top one, the top page folding back, the rings. */
const Calendar = ({ f, pages, times, durs, bare = false }: { f: number; pages: { day: number; kind: PageKind }[]; times: number[]; durs: number[]; bare?: boolean }) => {
  let k = 0;
  while (k < times.length && f >= times[k] + durs[k]) k++; // pages fully gone
  const cur = Math.min(k, pages.length - 1);
  const flipping = k < times.length && f >= times[k] ? (f - times[k]) / durs[k] : 0;
  const pFlip = Easing.in(Easing.quad)(Math.min(1, flipping));
  return (
    <div style={{ position: "absolute", left: CAL.x, top: CAL.y, width: CAL.w, height: CAL.h, perspective: 1600 }}>
      {/* The pad's thickness: a few page edges under the top page. */}
      {[3, 2, 1].map((i) => <div key={i} style={{ position: "absolute", left: i * 4, right: -i * 4, top: i * 7, bottom: -i * 7, borderRadius: 26, background: "#121316", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }} />)}
      {bare ? null : (
        <>
          {cur + 1 < pages.length ? <CalPage {...pages[cur + 1]} /> : null}
          {/* The top page folds back over the binding. It stays solid while it still covers the page
              under it and stays inside the frame, so two pages' type never show through each other. */}
          <div style={{ position: "absolute", inset: 0, transformOrigin: "50% 0%", transform: `translateY(${(-pFlip * 150).toFixed(1)}px) rotateX(${(-pFlip * 78).toFixed(2)}deg)`, opacity: pFlip >= 1 ? 0 : 1 - Math.max(0, pFlip - 0.8) / 0.2 }}>
            <CalPage {...pages[cur]} />
          </div>
        </>
      )}
      {/* Binding rings. */}
      {[0.28, 0.72].map((x) => <div key={x} style={{ position: "absolute", left: CAL.w * x - 14, top: -18, width: 28, height: 44, borderRadius: 14, background: "#2f3138", boxShadow: "inset 0 0 0 3px #3f424b" }} />)}
    </div>
  );
};

// ── The lockup: the lamp grille and the wordmark. `lamp(i)` is how lit lamp i is, in meter order
// (column by column, bottom up).
const MARK_W = 1606.1 / 1157; // the mark plus its gap, as a fraction of the lockup's height
const WORD_W = 4855 / 1157;
const lockupWidth = (size: number) => size * (MARK_W + WORD_W);
const lampSpot = (size: number, i: number) => {
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  const [col, row] = LAMP_ORDER[i];
  return { cx: rr + col * pitch, cy: rr + row * pitch, rr };
};
const Lockup = ({ x, y, size, grille = 1, lamp, word = 1, wordDx = 0, opacity = 1 }: { x: number; y: number; size: number; grille?: number; lamp: (i: number) => number; word?: number; wordDx?: number; opacity?: number }) => {
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  return (
    <div style={{ position: "absolute", left: x, top: y, width: lockupWidth(size), height: size, opacity }}>
      <svg width={size} height={size} style={{ position: "absolute", overflow: "visible" }}>
        {Array.from({ length: 16 }, (_, i) => <circle key={i} cx={rr + Math.floor(i / 4) * pitch} cy={rr + (i % 4) * pitch} r={rr} fill={C.grille} opacity={grille} />)}
        {LAMP_ORDER.map((_, i) => {
          const s = lampSpot(size, i);
          return <circle key={i} cx={s.cx} cy={s.cy} r={rr * lamp(i)} fill={C.lamp} />;
        })}
      </svg>
      <Img src={staticFile("logo/word.svg")} style={{ position: "absolute", left: size * MARK_W + wordDx, top: 0, width: size * WORD_W, height: size, opacity: word }} />
    </div>
  );
};

// ── Slide changes. Nothing cross-fades. Each change is one move, started on the beat:
//   · copy lifts out before the cut and the next slide's copy rises in after it;
//   · the panel is one surface for the whole film: it reshapes from one slide's panel to the next
//     (it splits in two and joins again) and the next slide's content comes up inside it;
//   · where a slide's subject carries over, it travels: the Wrapped page becomes the mark's first
//     lamp, the lockup shrinks into the corner, the #1 row's cover grows into the artist slide,
//     the browser window folds into the mark.
const enter = (f: number, cut: number) => r(f, cut + 1, 9, E.out);
const leave = (f: number, cut: number) => r(f, cut - 5, 5, E.in);
const morph = (f: number, cut: number) => r(f, cut - 2, 14, E.emph);

/** A slide's copy, outside the panel: rises in after the cut at `a`, lifts out before the cut at `b`. */
const Copy = ({ f, a, b, style, children }: { f: number; a: number; b: number; style?: React.CSSProperties; children: React.ReactNode }) => {
  if (f < a || f >= b) return null;
  // Copy waits a few frames longer than the panel's content: by then the reshaping surface has
  // moved clear of where the copy lands, so the two never cross.
  const i = r(f, a + 5, 8, E.out);
  const o = leave(f, b);
  return <div style={{ position: "absolute", opacity: i * (1 - o), transform: `translateY(${((1 - i) * 28 - o * 20).toFixed(1)}px)`, ...style }}>{children}</div>;
};
/** A slide's content inside the surface: it fades up once the surface has taken its shape. */
const Inside = ({ f, a, b, children }: { f: number; a: number; b: number; children: React.ReactNode }) => {
  if (f < a || f >= b) return null;
  return <div style={{ position: "absolute", inset: 0, opacity: enter(f, a) * (1 - leave(f, b)) }}>{children}</div>;
};

type Rect = { x: number; y: number; w: number; h: number; r: number };
const lerpRect = (a: Rect, b: Rect, t: number): Rect => ({ x: mix(a.x, b.x, t), y: mix(a.y, b.y, t), w: mix(a.w, b.w, t), h: mix(a.h, b.h, t), r: mix(a.r, b.r, t) });
const END_LOCK = { size: 150, y: 232 };
const RECT = {
  server: { x: 120, y: 540, w: 1680, h: 300, r: 28 },
  top: { x: 880, y: 120, w: 920, h: 840, r: 22 },
  artist: { x: 880, y: 360, w: 920, h: 600, r: 22 },
  sessions: { x: 120, y: 360, w: 1680, h: 600, r: 22 },
  taste: { x: 880, y: 150, w: 920, h: 780, r: 22 },
  recent: { x: 120, y: 330, w: 820, h: 640, r: 22 },
  importing: { x: 980, y: 330, w: 820, h: 640, r: 22 },
  window: { x: 120, y: 320, w: 1000, h: 597, r: 16 },
  mark: { x: 960 - lockupWidth(END_LOCK.size) / 2, y: END_LOCK.y, w: END_LOCK.size, h: END_LOCK.size, r: 36 },
} satisfies Record<string, Rect>;
const surfaceStyle = (rc: Rect): React.CSSProperties => ({ position: "absolute", left: rc.x, top: rc.y, width: rc.w, height: rc.h, borderRadius: rc.r, background: PANEL, boxShadow: `0 0 0 1.5px ${EDGE}`, overflow: "hidden" });

// ── 1. The hook. A tear-off calendar: the year's pages flip by blank and land on the one page
// with anything on it, in December: Wrapped.
// ── 3. The answer: not once a year, every single day. The year again, and this time every page is
// lit and full, faster and faster; then the pad drops into your server.
const Year = ({ f }: { f: number }) => {
  const intro = CUT.intro;
  const hist = CUT.history;
  const sv = CUT.server;
  const b = CUT.top;
  if (f >= b) return null;

  // Act 1: blank pages Jan → Nov, landing on Dec 1, Wrapped.
  const blankDays = Array.from({ length: 8 }, (_, i) => Math.round((i / 8) * (WRAPPED_DAY - 30)));
  const act1 = [...blankDays.map((day) => ({ day, kind: "blank" as PageKind })), { day: WRAPPED_DAY, kind: "wrapped" as PageKind }];
  const s1 = schedule(16, w("once") - 2, act1.length - 1, 1);
  // Act 2: Jan 1 onwards, every page full, done before the pad moves.
  const fullDays = Array.from({ length: 20 }, (_, i) => Math.round((i / 19) * 364));
  const act2 = fullDays.map((day) => ({ day, kind: "full" as PageKind }));
  const s2 = schedule(hist + 12, sv - 10, act2.length - 1, 0.6); // slow at first, then faster and faster

  // Into the intro slide: the Wrapped page leaves the pad and becomes the mark's first lamp
  // (see Intro); the bare pad slides away.
  const padOut = r(f, intro - 2, 8, E.in);
  // Into the history slide: a full pad slides in from the right.
  const padIn = r(f, hist, 12, E.out);
  // Into the server, in strict turns: the copy clears, the pad shrinks up out of the way, the
  // server rises in under it (the surface), the pad drops through the slot.
  const shrink = r(f, sv - 8, 10, E.inOut);
  const drop = r(f, sv + 8, 12, E.in);
  const SLOT_Y = RECT.server.y;
  const copyOut = r(f, sv - 8, 5, E.in);

  return (
    <AbsoluteFill>
      {f < intro + 6 ? (
        <div style={{ position: "absolute", inset: 0, opacity: 1 - padOut, transform: `translateX(${(padOut * 90).toFixed(1)}px)` }}>
          <Calendar f={f} pages={act1} times={s1.times} durs={s1.durs} bare={f >= intro - 2} />
        </div>
      ) : null}
      {f >= hist ? (
        <div style={{ position: "absolute", inset: 0, clipPath: drop > 0 ? `inset(0 0 ${1080 - SLOT_Y}px 0)` : undefined }}>
          <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, opacity: padIn, transform: `translate(${((1 - padIn) * 160).toFixed(1)}px, ${(mix(0, -220, shrink) + drop * 600).toFixed(1)}px) scale(${mix(1, 0.5, shrink).toFixed(4)})`, transformOrigin: `${CAL.x + CAL.w / 2}px ${CAL.y + CAL.h / 2}px` }}>
            <Calendar f={f} pages={act2} times={s2.times} durs={s2.durs} />
          </div>
        </div>
      ) : null}
      {/* Copy, left column, in strict turns: each block is gone before the next arrives. */}
      <Say
        f={f}
        size={88}
        out={intro - 6}
        style={{ left: 120, top: 400, width: 1040 }}
        words={[
          { t: "Spotify", at: 0 },
          { t: "Wrapped", at: 0 },
          { t: "\n", at: 0 },
          { t: "comes", at: w("comes") },
          { t: "once", at: 0 },
          { t: "a", at: 0 },
          { t: "year.", at: 0 },
        ]}
      />
      {f >= hist && f < sv - 3 ? (
        <div style={{ position: "absolute", left: 120, top: 434, width: 1040, opacity: enter(f, hist) * (1 - copyOut), transform: `translateY(${((1 - enter(f, hist)) * 28).toFixed(1)}px)` }}>
          {/* The answer to the opener, in the same place and size as "comes once a year". */}
          <div style={{ ...headStyle(88) }}>
            <span style={{ color: C.mist }}>Not once a year.</span>
            <br />
            <span style={{ color: C.lamp, opacity: r(f, w("every") - 1, 8) }}>Every single day.</span>
          </div>
          <div style={{ ...subLine, fontFamily: FONT, fontSize: 46, marginTop: 30, opacity: r(f, w("history") - 1, 10), transform: `translateY(${((1 - r(f, w("history") - 1, 10)) * 12).toFixed(1)}px)` }}>Your whole listening history.</div>
        </div>
      ) : null}
      <Copy f={f} a={sv - 3} b={b} style={{ left: 120, top: 370, ...headStyle(96) }}>
        On your own server.
      </Copy>
    </AbsoluteFill>
  );
};

// ── 2. The introduction: what this is, said once ("This is spotussi. Free, open-source stats for
// your Spotify."). The Wrapped page shrinks into the first lamp of the mark, the rest of the meter
// lights, the name and the plain line arrive as she says them. Then the lockup
// travels to the corner of the history slide and stays there while the year flips by.
const INTRO = { size: 190, y: 372 };
const SMALL = { size: 64, x: 120, y: 330 };
const Intro = ({ f }: { f: number }) => {
  const a = CUT.intro;
  const hist = CUT.history;
  const sv = CUT.server;
  if (f < a - 2 || f >= sv - 3) return null;
  const big = { x: 960 - lockupWidth(INTRO.size) / 2, y: INTRO.y, size: INTRO.size };
  const first = lampSpot(big.size, 0);
  // The page → the first lamp.
  const fly = r(f, a - 2, 11, E.emph);
  const page: Rect = { x: CAL.x, y: CAL.y, w: CAL.w, h: CAL.h, r: 26 };
  const dot: Rect = { x: big.x + first.cx - first.rr, y: big.y + first.cy - first.rr, w: first.rr * 2, h: first.rr * 2, r: first.rr };
  const rc = lerpRect(page, dot, fly);
  const landed = a + 9;
  const name = w("brand") - 3; // "This is" lights the meter; the wordmark arrives with the name
  const line = w("free") - 1; // and the plain line as she says it
  // The lockup → the corner of the history slide.
  const shrink = morph(f, hist);
  const size = mix(big.size, SMALL.size, shrink);
  return (
    <>
      <Lockup
        x={mix(big.x, SMALL.x, shrink)}
        y={mix(big.y, SMALL.y, shrink)}
        size={size}
        grille={r(f, a + 2, 7)}
        lamp={(i) => (i === 0 ? (f >= landed ? 1 : 0) : r(f, landed + i * 1.1, 4))}
        word={r(f, name, 10)}
        wordDx={(1 - r(f, name, 12)) * -28}
        opacity={1 - r(f, sv - 8, 5, E.in)}
      />
      {f < landed ? (
        <div style={{ position: "absolute", left: rc.x, top: rc.y, width: rc.w, height: rc.h, borderRadius: rc.r, overflow: "hidden", background: interpolateColors(fly, [0, 0.6], ["#0f2a19", C.lamp]) }}>
          <div style={{ position: "absolute", left: 0, top: 0, width: CAL.w, height: CAL.h, opacity: 1 - r(f, a - 2, 4, E.in) }}>
            <CalPage day={WRAPPED_DAY} kind="wrapped" />
          </div>
        </div>
      ) : null}
      {f < hist ? (
        <div style={{ position: "absolute", left: 0, right: 0, top: INTRO.y + INTRO.size + 74, textAlign: "center", ...headStyle(60), opacity: r(f, line, 10) * (1 - leave(f, hist)), transform: `translateY(${((1 - r(f, line, 12)) * 24 - leave(f, hist) * 20).toFixed(1)}px)` }}>
          Free, <span style={{ color: C.lamp }}>open-source</span> stats for your Spotify.
        </div>
      ) : null}
    </>
  );
};

/** A second, quieter line under a headline, for the sentence that follows it. */
const subLine: React.CSSProperties = { position: "relative", color: C.mist, fontStretch: "100%", fontWeight: 520, letterSpacing: "0" };

// ── The server: the surface's first shape. Its front panel is the mark.
const ServerInside = ({ f }: { f: number }) => {
  const lamps = r(f, CUT.server + 20, 6);
  return (
    <>
      <div style={{ position: "absolute", left: CAL.x + CAL.w / 2 - RECT.server.x - 170, top: -2, width: 340, height: 12, borderRadius: 6, background: "#050506" }} />
      <svg width={180} height={180} style={{ position: "absolute", left: 70, top: 60 }}>
        {Array.from({ length: 16 }, (_, i) => <circle key={i} cx={lampSpot(180, 0).rr + Math.floor(i / 4) * (180 / 3.786)} cy={lampSpot(180, 0).rr + (i % 4) * (180 / 3.786)} r={lampSpot(180, 0).rr} fill={C.grille} />)}
        {LAMP_ORDER.map((_, i) => {
          const s = lampSpot(180, i);
          return <circle key={i} cx={s.cx} cy={s.cy} r={s.rr} fill={C.lamp} opacity={lamps} />;
        })}
      </svg>
      <div style={{ position: "absolute", left: 320, top: 86 }}>
        <div style={{ ...headStyle(64) }}>spotussi.local</div>
        <div style={{ fontFamily: FONT, fontSize: 30, color: MUTE, marginTop: 16 }}>Your history, on your hardware.</div>
      </div>
    </>
  );
};

// ── 4. Your real top 10: the list starts on "This month"; on "Not just this month's" the switch
// slides to "All time" and the all-time #1 rises to the top.
const ARTISTS = [
  { name: "Nova Reyes", all: 12418, month: 88, seed: 1 },
  { name: "The Midnight Arcade", all: 9870, month: 41, seed: 2 },
  { name: "Kairo", all: 8402, month: 118, seed: 3 },
  { name: "Lumen Drift", all: 7955, month: 33, seed: 4 },
  { name: "Static Bloom", all: 6310, month: 142, seed: 5 },
  { name: "June Avenue", all: 5722, month: 97, seed: 6 },
  { name: "Halcyon Coast", all: 4960, month: 29, seed: 7 },
  { name: "Mira Vale", all: 4415, month: 63, seed: 8 },
  { name: "Paper Satellites", all: 3980, month: 52, seed: 9 },
  { name: "Oslo Nights", all: 3512, month: 74, seed: 10 },
];
const TOP_ROW = 66;
const TOP_SLOT = { left: 28, top: 108, pad: 12 };
const TopRow = ({ x, rank, val, frac, lit = false, rankOpacity = 1, hideArt = false }: { x: (typeof ARTISTS)[number]; rank: number; val: number; frac: number; lit?: boolean; rankOpacity?: number; hideArt?: boolean }) => (
  <div style={{ height: TOP_ROW, padding: `0 ${TOP_SLOT.pad}px`, display: "flex", alignItems: "center", gap: 20 }}>
    <div style={{ width: 36, textAlign: "right", ...headStyle(26), color: lit ? C.lamp : MUTE, opacity: rankOpacity }}>{rank}</div>
    <Art seed={x.seed} size={46} round style={{ visibility: hideArt ? "hidden" : undefined }} />
    <div style={{ width: 290, fontFamily: FONT, fontSize: 24, fontWeight: 620, color: C.paper, whiteSpace: "nowrap" }}>{x.name}</div>
    <div style={{ flex: 1, height: 8, borderRadius: 4, background: "#23252b" }}>
      <div style={{ width: `${(frac * 100).toFixed(2)}%`, height: 8, borderRadius: 4, background: C.lamp }} />
    </div>
    <div style={{ width: 110, textAlign: "right", fontFamily: FONT, fontSize: 24, fontWeight: 680, color: C.paper, fontVariantNumeric: "tabular-nums" }}>{fmt(val)}</div>
  </div>
);
const BY_MONTH = [...ARTISTS].sort((x, y) => y.month - x.month);
const BY_ALL = [...ARTISTS].sort((x, y) => y.all - x.all);
const LEAD_ARTIST = BY_ALL[0];
const TopInside = ({ f }: { f: number }) => {
  const sw = w("notJust") - 3; // the switch starts a breath before "Not just this month's"
  const pill = r(f, sw, 14, E.inOut);
  const maxM = BY_MONTH[0].month;
  const maxA = BY_ALL[0].all;
  // One lead: the all-time #1 rises to the top as a solid strip, its count running up. Nothing
  // else travels. The rows in its path clear first; the rest of the month's rows dim and hold
  // still; once the lead has passed, each slot takes its all-time artist, top to bottom, old out
  // before new in. No row ever crosses another.
  const leadFrom = BY_MONTH.indexOf(LEAD_ARTIST);
  const rise = r(f, sw + 2, 14, E.inOut);
  const swap = (i: number) => sw + 14 + i; // the frame slot i takes its all-time artist
  const slot = (i: number): React.CSSProperties => ({ position: "absolute", left: TOP_SLOT.left, width: RECT.top.w - TOP_SLOT.left * 2, top: TOP_SLOT.top + i * TOP_ROW });
  return (
    <>
      <div style={{ position: "absolute", left: 40, top: 36, ...headStyle(40) }}>Top artists</div>
      {/* The range switch slides from This month to All time. */}
      <div style={{ position: "absolute", left: RECT.top.w - 440, top: 26, width: 400, height: 60, borderRadius: 30, background: "#0d0e11", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
        <div style={{ position: "absolute", top: 6, left: mix(6, 200, pill), width: 194, height: 48, borderRadius: 24, background: "#2a2c33" }} />
        {["This month", "All time"].map((l, i) => (
          <div key={l} style={{ position: "absolute", top: 0, left: i ? 200 : 6, width: 194, lineHeight: "60px", textAlign: "center", fontFamily: FONT, fontSize: 24, fontWeight: 640, color: (i ? pill : 1 - pill) > 0.5 ? C.paper : MUTE }}>{l}</div>
        ))}
      </div>
      {BY_MONTH.map((x, i) =>
        x === LEAD_ARTIST || f >= swap(i) ? null : (
          <div key={`m-${x.name}`} style={{ ...slot(i), opacity: i < leadFrom ? 1 - r(f, sw, 5, E.in) : mix(1, 0.3, r(f, sw, 4)) * (1 - r(f, swap(i) - 3, 3, E.in)) }}>
            <TopRow x={x} rank={i + 1} val={x.month} frac={x.month / maxM} />
          </div>
        ),
      )}
      {BY_ALL.map((x, i) =>
        i === 0 || f < swap(i) ? null : (
          <div key={`a-${x.name}`} style={{ ...slot(i), opacity: r(f, swap(i), 6) }}>
            <TopRow x={x} rank={i + 1} val={x.all} frac={x.all / maxA} />
          </div>
        ),
      )}
      <div style={{ ...slot(mix(leadFrom, 0, rise)), background: PANEL, zIndex: 2 }}>
        {/* The lead's cover is handed to the travelling cover just before the cut (see HeroCover). */}
        <TopRow x={LEAD_ARTIST} rank={rise < 0.5 ? leadFrom + 1 : 1} rankOpacity={Math.abs(1 - 2 * rise)} lit={rise >= 0.5} val={mix(LEAD_ARTIST.month, LEAD_ARTIST.all, r(f, sw + 2, 22, E.out))} frac={mix(LEAD_ARTIST.month / maxM, 1, rise)} hideArt={f >= CUT.artist - 5} />
      </div>
    </>
  );
};
/** The #1 artist's cover travels: from its row at the top of the list to the artist slide, growing as it goes. */
const HeroCover = ({ f }: { f: number }) => {
  const a = CUT.artist;
  const b = CUT.sessions;
  if (f < a - 5 || f >= b) return null;
  const t = morph(f, a);
  const o = leave(f, b);
  const from = { x: RECT.top.x + TOP_SLOT.left + TOP_SLOT.pad + 36 + 20, y: RECT.top.y + TOP_SLOT.top + (TOP_ROW - 46) / 2, s: 46 };
  const size = mix(from.s, 330, t);
  return (
    <div style={{ position: "absolute", left: mix(from.x, 120, t), top: mix(from.y, 360, t), opacity: 1 - o, transform: `translateY(${(-o * 20).toFixed(1)}px)` }}>
      <Art seed={LEAD_ARTIST.seed} size={size} round />
    </div>
  );
};

// ── 5. Your #1 artist: who it is, how many plays all time, and since when.
const YEARLY = [0.18, 0.22, 0.3, 0.26, 0.41, 0.48, 0.55, 0.72, 0.95, 0.8, 0.67, 0.74, 0.61, 0.7, 0.83, 0.58];
const ArtistInside = ({ f }: { f: number }) => {
  const a = CUT.artist;
  // The count runs from the moment the slide arrives and settles as she says "twelve thousand
  // plays", so the panel never sits on a zero.
  const count = r(f, a, w("twelve") + 14 - a, E.out);
  const first = r(f, w("since") - 1, 10);
  return (
    <>
      <div style={{ position: "absolute", left: 44, top: 40 }}>
        <Label style={{ fontSize: 24 }}>Plays, all time</Label>
        <div style={{ ...headStyle(150), marginTop: 6, fontVariantNumeric: "tabular-nums", letterSpacing: "-0.05em" }}>{fmt(12418 * count)}</div>
      </div>
      <div style={{ position: "absolute", left: 44, top: 248, display: "flex", alignItems: "baseline", gap: 18, opacity: first }}>
        <Label style={{ fontSize: 24 }}>First play</Label>
        <div style={{ ...headStyle(44) }}>Mar 4, 2011</div>
      </div>
      <Label style={{ position: "absolute", left: 44, top: 336, fontSize: 22 }}>Plays per year</Label>
      <div style={{ position: "absolute", left: 44, top: 540, width: 832, height: 2, background: EDGE }} />
      {YEARS.map((y, i) => {
        const x = 44 + (i / (YEARS.length - 1)) * 798;
        const h = 150 * YEARLY[i];
        return (
          <div key={y} style={{ position: "absolute", left: x, top: 540 - h, width: 34, height: h, borderRadius: "6px 6px 0 0", background: C.lamp, opacity: 0.5 + 0.5 * (i / (YEARS.length - 1)) }}>
            {i % 5 === 0 ? <div style={{ position: "absolute", top: h + 12, left: -14, width: 62, textAlign: "center", fontFamily: FONT, fontSize: 20, color: MUTE }}>{y}</div> : null}
          </div>
        );
      })}
    </>
  );
};

// ── 6. Sessions: three days, two sittings each, every play a cover. A playhead marker runs along
// each lane and the plays light as it passes.
const SESSIONS = [
  { day: "Mon", blocks: [{ x: 0, n: 6, span: "07:48 – 08:09" }, { x: 760, n: 9, span: "18:20 – 18:51" }] },
  { day: "Wed", blocks: [{ x: 300, n: 8, span: "12:05 – 12:33" }, { x: 1040, n: 5, span: "21:40 – 21:57" }] },
  { day: "Sat", blocks: [{ x: 120, n: 9, span: "10:30 – 11:02" }, { x: 900, n: 7, span: "16:15 – 16:39" }] },
];
const SessionsInside = ({ f }: { f: number }) => {
  const a = CUT.sessions;
  const b = CUT.taste;
  const X0 = 150; // where the lanes start inside the panel
  const W = 1490;
  const TILE = 64;
  const headX = X0 + r(f, a + 10, b - a - 24, (t) => t) * W;
  return (
    <>
      <div style={{ position: "absolute", left: 40, top: 30, ...headStyle(36) }}>Sessions</div>
      {SESSIONS.map((lane, li) => (
        <div key={lane.day} style={{ position: "absolute", left: 0, width: RECT.sessions.w, top: 112 + li * 158 }}>
          <div style={{ position: "absolute", left: 40, top: 26, fontFamily: FONT, fontSize: 26, fontWeight: 600, color: MUTE }}>{lane.day}</div>
          {/* The playhead: one marker per lane, moving as one. It runs across the plays, not the labels under them. */}
          <div style={{ position: "absolute", top: -8, height: 100, left: headX, width: 3, borderRadius: 2, background: C.lamp, zIndex: 1 }} />
          {lane.blocks.map((bk, k) => (
            <div key={k} style={{ position: "absolute", left: X0 + bk.x, top: 0 }}>
              <div style={{ display: "flex", gap: 8, padding: 10, borderRadius: 16, background: "#1d1f24", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
                {Array.from({ length: bk.n }, (_, p) => {
                  const left = X0 + bk.x + 10 + p * (TILE + 8);
                  return <Art key={p} seed={(li * 11 + k * 5 + p * 3) % 40} size={TILE} style={{ opacity: mix(0.14, 1, r(headX, left, 30, (t) => t)) }} />;
                })}
              </div>
              <div style={{ marginTop: 10, marginLeft: 10, fontFamily: FONT, fontSize: 24, fontWeight: 520, color: MUTE, whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
                {bk.span} · {bk.n} plays
              </div>
            </div>
          ))}
        </div>
      ))}
    </>
  );
};

// ── 7. Taste: one stacked bar per year of the history, whose genre bands shift as the years run;
// one genre grows.
const GENRES = [
  { name: "Indie", col: "#5b5f68" },
  { name: "House", col: "#3a3d46" },
  { name: "Synthpop", col: C.lamp },
  { name: "Hip hop", col: "#80848e" },
];
const TasteInside = ({ f }: { f: number }) => {
  const a = CUT.taste;
  const b = CUT.live;
  const N = YEARS.length;
  const H = 500;
  const PITCH = 44;
  const BAR = 32;
  const base = RECT.taste.h - 84; // the bars stand on this line; the years sit under it
  const shift = r(f, a + 10, b - a - 20, E.inOut); // starts once the slide has arrived
  const synAt = (week: number) => 0.12 + 0.55 * Math.min(1, Math.max(0, (week / (N - 1)) * shift * 1.25));
  const lastSyn = synAt(N - 1);
  return (
    <>
      <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(36) }}>Genres over time</div>
      <div style={{ position: "absolute", left: 40, top: 96, display: "flex", gap: 26 }}>
        {GENRES.map((g) => (
          <span key={g.name} style={{ display: "flex", alignItems: "center", gap: 9, fontFamily: FONT, fontSize: 22, color: g.col === C.lamp ? C.paper : MUTE }}>
            <span style={{ width: 14, height: 14, borderRadius: 7, background: g.col }} />
            {g.name}
          </span>
        ))}
      </div>
      {Array.from({ length: N }, (_, week) => {
        const syn = synAt(week);
        const rest = 1 - syn;
        const parts = [rest * 0.42, rest * 0.33, syn, rest * 0.25];
        return (
          <div key={week} style={{ position: "absolute", left: 40 + week * PITCH, top: base - H, width: BAR }}>
            <div style={{ height: H, borderRadius: 8, overflow: "hidden", display: "flex", flexDirection: "column-reverse" }}>
              {parts.map((p, k) => <div key={k} style={{ height: `${(p * 100).toFixed(2)}%`, background: GENRES[k].col, borderTop: "2px solid #15161a" }} />)}
            </div>
            {week % 5 === 0 ? <div style={{ marginTop: 12, marginLeft: -14, width: 60, textAlign: "center", fontFamily: FONT, fontSize: 20, color: MUTE }}>{YEARS[week]}</div> : null}
          </div>
        );
      })}
      {/* The growing band names itself: the label rides the middle of the last week's green band. */}
      <div style={{ position: "absolute", left: 40 + (N - 1) * PITCH + BAR + 18, top: base - ((1 - lastSyn) * 0.75 + lastSyn / 2) * H - 17, fontFamily: FONT, fontSize: 28, fontWeight: 680, lineHeight: "34px", color: C.lamp }}>Synthpop</div>
    </>
  );
};

// ── 8. Live and imported, one after the other. The surface splits in two: a new play opens a row
// at the top of Recently played ("as it happens"); then the export imports and the whole past
// counts up ("everything before").
const RECENT = [
  { t: "Glass Hours", a: "Nova Reyes", s: 11 },
  { t: "Night Signal", a: "Kairo", s: 13 },
  { t: "Paper Moons", a: "Static Bloom", s: 15 },
  { t: "Low Tide", a: "June Avenue", s: 16 },
];
const PlayRow = ({ t, a, s, when, lit = false }: { t: string; a: string; s: number; when: string; lit?: boolean }) => (
  <div style={{ height: 80, display: "flex", alignItems: "center", gap: 20, padding: "0 12px" }}>
    <Art seed={s} size={58} />
    <div style={{ flex: 1 }}>
      <div style={{ fontFamily: FONT, fontSize: 25, fontWeight: 640, color: C.paper }}>{t}</div>
      <div style={{ fontFamily: FONT, fontSize: 19, color: MUTE }}>{a}</div>
    </div>
    <div style={{ fontFamily: FONT, fontSize: 19, color: lit ? C.lamp : DIM }}>{when}</div>
  </div>
);
const RecentInside = ({ f }: { f: number }) => {
  const drop = r(f, w("asItHappens") - 2, 14, E.soft);
  const rowW = RECT.recent.w - 72;
  return (
    <>
      <div style={{ position: "absolute", left: 36, top: 30, display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ width: 14, height: 14, borderRadius: 7, background: C.lamp }} />
        <span style={{ ...headStyle(32) }}>Recently played</span>
      </div>
      {/* The new play opens its own row: the list moves down exactly as far as the row has grown,
          so it never sits on top of another row. */}
      <div style={{ position: "absolute", left: 36, width: rowW, top: 100, height: 80 * drop, borderRadius: 14, overflow: "hidden", background: "#162a21", opacity: drop }}>
        <PlayRow t="Neon Harbor" a="The Midnight Arcade" s={21} when="now" lit />
      </div>
      {RECENT.map((x, i) => (
        <div key={x.t} style={{ position: "absolute", left: 36, width: rowW, top: 100 + (i + drop) * 92 }}>
          <PlayRow t={x.t} a={x.a} s={x.s} when={`${(i + 1) * 3} min ago`} />
        </div>
      ))}
    </>
  );
};
const ImportInside = ({ f }: { f: number }) => {
  const imp = w("imports");
  const fill = r(f, imp, 28, E.inOut);
  const shown = r(f, imp, 6);
  const inner = RECT.importing.w - 80;
  return (
    <>
      <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(32) }}>Import your history</div>
      <div style={{ position: "absolute", left: 40, top: 110, width: inner, height: 140, borderRadius: 18, boxShadow: `inset 0 0 0 2px ${EDGE}`, display: "flex", alignItems: "center", gap: 22, padding: "0 28px", boxSizing: "border-box" }}>
        <div style={{ width: 74, height: 90, borderRadius: 10, background: "#22242a", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, display: "flex", alignItems: "flex-end", justifyContent: "center", paddingBottom: 10, boxSizing: "border-box", fontFamily: FONT, fontSize: 16, fontWeight: 700, color: MUTE }}>ZIP</div>
        <div>
          <div style={{ fontFamily: FONT, fontSize: 24, fontWeight: 640, color: C.paper }}>Extended streaming history</div>
          <div style={{ fontFamily: FONT, fontSize: 19, color: MUTE }}>from Spotify's data export</div>
        </div>
      </div>
      <div style={{ position: "absolute", left: 40, width: inner, top: 300, height: 12, borderRadius: 6, background: "#23252b" }}>
        <div style={{ width: `${(fill * 100).toFixed(2)}%`, height: 12, borderRadius: 6, background: C.lamp }} />
      </div>
      {/* Nothing is shown until the import starts: no zero waiting on screen. */}
      <div style={{ position: "absolute", left: 40, top: 340, ...headStyle(76), fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap", opacity: shown }}>
        {fmt(184213 * fill)}
        <span style={{ ...headStyle(30), color: MUTE, marginLeft: 18 }}>plays since 2011</span>
      </div>
      {/* The year slots are there from the start, dark, and fill green as the import runs. */}
      {YEARS.map((y, i) => {
        const hh = 40 + 90 * (0.35 + (0.65 * ((i * 37) % 11)) / 10);
        const on = Math.min(1, Math.max(0, fill * YEARS.length - i));
        return (
          <div key={y} style={{ position: "absolute", left: 40 + i * 46, top: RECT.importing.h - 24 - 166, width: 32 }}>
            <div style={{ position: "relative", height: 130 }}>
              <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: hh, borderRadius: "6px 6px 0 0", background: "#1d1f24", overflow: "hidden" }}>
                <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: hh * on, background: C.lamp }} />
              </div>
            </div>
            <div style={{ height: 36, marginLeft: -14, width: 60, paddingTop: 10, textAlign: "center", fontFamily: FONT, fontSize: 18, color: MUTE }}>{i % 5 === 0 ? y : ""}</div>
          </div>
        );
      })}
    </>
  );
};

// ── 9. Three ways in, one after the other. The two panels join into the browser window (the web
// app, the hero); the phone slides in on "In your pocket"; then your agent: spotussi has an MCP
// server, so a question in plain words gets an answer from the same data.
const WIN = { w: 1240, h: 740 }; // the dashboard is drawn at this size and scaled to the window
const WindowInside = () => (
  <div style={{ position: "absolute", left: 0, top: 0, width: WIN.w, height: WIN.h, background: "#101114", transform: `scale(${(RECT.window.w / WIN.w).toFixed(4)})`, transformOrigin: "0 0" }}>
    <div style={{ height: 42, background: "#17181c", display: "flex", alignItems: "center", gap: 8, paddingLeft: 18 }}>
      {[0, 1, 2].map((i) => <span key={i} style={{ width: 12, height: 12, borderRadius: 6, background: C.grille }} />)}
      <span style={{ marginLeft: 420, padding: "4px 60px", borderRadius: 7, background: "#0b0c0f", color: MUTE, fontFamily: FONT, fontSize: 14 }}>spotussi.local</span>
    </div>
    <div style={{ position: "absolute", left: 30, top: 70, width: WIN.w - 60, display: "flex", gap: 20 }}>
      {[
        { l: "Time listened", n: 136, u: "h" },
        { l: "Plays", n: 2341 },
        { l: "Artists", n: 568 },
        { l: "Tracks", n: 1923 },
      ].map((x, i) => (
        <div key={x.l} style={{ flex: 1, height: 140, boxSizing: "border-box", borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: "18px 20px" }}>
          <Label>{x.l}</Label>
          <div style={{ ...headStyle(44), marginTop: 8, fontVariantNumeric: "tabular-nums" }}>
            {fmt(x.n)}
            {x.u ?? ""}
          </div>
          <div style={{ display: "flex", gap: 5, marginTop: 10 }}>
            {Array.from({ length: 14 }, (_, k) => <span key={k} style={{ width: 9, height: 9, borderRadius: 5, background: k < 6 + i * 2 ? C.lamp : C.grille }} />)}
          </div>
        </div>
      ))}
    </div>
    <div style={{ position: "absolute", left: 30, top: 240, width: 760, height: 480, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
      <Label style={{ position: "absolute", left: 20, top: 18 }}>Plays by day</Label>
      {Array.from({ length: 28 }, (_, i) => {
        const hgt = 80 + 280 * Math.abs(Math.sin(i * 1.7) * 0.6 + Math.sin(i * 0.4) * 0.4);
        return <div key={i} style={{ position: "absolute", left: 24 + i * 26, bottom: 24, width: 16, height: hgt, borderRadius: 4, background: C.lamp, opacity: i === 18 ? 1 : 0.62 }} />;
      })}
    </div>
    <div style={{ position: "absolute", left: 810, top: 240, width: 400, height: 480, boxSizing: "border-box", borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: 20 }}>
      <Label>Top artists</Label>
      {ARTISTS.slice(0, 5).map((x) => (
        <div key={x.name} style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 20 }}>
          <Art seed={x.seed} size={44} round />
          <span style={{ fontFamily: FONT, fontSize: 20, fontWeight: 620, color: C.paper }}>{x.name}</span>
        </div>
      ))}
    </div>
  </div>
);

// ── The surface. One panel for the whole film, from the server to the sign-off: it takes each
// slide's shape in turn. For "live and imported" a second panel splits off it and joins it again.
const PHONE = { x: 1150, y: 354, w: 250 };
const AGENT: Rect = { x: 1430, y: 320, w: 370, h: 597, r: 22 };
const END_OUT = 5; // the last slide clears this many frames early, so the lockup lights on the downbeat
const Surface = ({ f }: { f: number }) => {
  const sv = CUT.server;
  if (f < sv || f >= CUT.end + 4) return null;
  const fold = r(f, CUT.end - END_OUT - 1, 9, E.emph); // the emptied window folds into the mark
  let rc: Rect = RECT.server;
  rc = lerpRect(rc, RECT.top, morph(f, CUT.top));
  rc = lerpRect(rc, RECT.artist, morph(f, CUT.artist));
  rc = lerpRect(rc, RECT.sessions, morph(f, CUT.sessions));
  rc = lerpRect(rc, RECT.taste, morph(f, CUT.taste));
  rc = lerpRect(rc, RECT.recent, morph(f, CUT.live));
  rc = lerpRect(rc, RECT.window, morph(f, CUT.devices));
  rc = lerpRect(rc, RECT.mark, fold);
  const rise = r(f, sv, 12);
  // The second panel: out from behind the first on the cut to "live", back into it on the next cut.
  let rb: Rect = lerpRect(RECT.taste, RECT.importing, morph(f, CUT.live));
  rb = lerpRect(rb, RECT.window, morph(f, CUT.devices));
  const second = f >= CUT.live - 2 && f < CUT.devices + 12;
  return (
    <>
      {second ? (
        <div style={{ ...surfaceStyle(rb), opacity: 1 - r(f, CUT.devices + 2, 8) }}>
          {/* Its content waits until the two panels have parted. */}
          <Inside f={f} a={CUT.live + 6} b={CUT.devices}>
            <ImportInside f={f} />
          </Inside>
        </div>
      ) : null}
      <div style={{ ...surfaceStyle(rc), opacity: r(f, sv, 6) * (1 - r(f, CUT.end - 3, 5)), transform: `translateY(${((1 - rise) * 40).toFixed(1)}px)` }}>
        <Inside f={f} a={sv - 3} b={CUT.top}>
          <ServerInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.top} b={CUT.artist}>
          <TopInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.artist} b={CUT.sessions}>
          <ArtistInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.sessions} b={CUT.taste}>
          <SessionsInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.taste} b={CUT.live}>
          <TasteInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.live} b={CUT.devices}>
          <RecentInside f={f} />
        </Inside>
        <Inside f={f} a={CUT.devices} b={CUT.end - END_OUT}>
          <WindowInside />
        </Inside>
      </div>
    </>
  );
};

/** Each slide's copy outside the surface. */
const Captions = ({ f }: { f: number }) => {
  const col: React.CSSProperties = { left: 120, top: 400, width: 740, display: "flex", flexDirection: "column", gap: 30 };
  const pocket = w("pocket");
  const out = CUT.end - END_OUT;
  const ph = r(f, pocket - 8, 18, E.land);
  const phOut = leave(f, out);
  const agent = w("agent") - 2; // "or just ask your agent"
  const way: React.CSSProperties = { top: 944, textAlign: "center", fontFamily: FONT, fontSize: 28, fontWeight: 560, color: C.mist };
  return (
    <>
      <Copy f={f} a={CUT.top} b={CUT.artist} style={col}>
        <div style={headStyle(92)}>
          Your <span style={{ color: C.lamp }}>real</span>
          <br />
          top 10.
        </div>
        <Say f={f} size={46} style={subLine} words={[{ t: "Not", at: w("notJust") }, { t: "just", at: 0 }, { t: "this", at: 0 }, { t: "month's.", at: 0 }]} />
      </Copy>

      <Copy f={f} a={CUT.artist} b={CUT.sessions} style={{ left: 120, top: 120, ...headStyle(110) }}>
        Your <span style={{ color: C.lamp }}>#1</span> artist.
      </Copy>
      <HeroCover f={f} />
      <Copy f={f} a={CUT.artist + 3} b={CUT.sessions} style={{ left: 120, top: 730 }}>
        <div style={headStyle(76)}>Nova Reyes</div>
        <div style={{ fontFamily: FONT, fontSize: 30, color: MUTE, marginTop: 10 }}>Synthpop</div>
      </Copy>

      <Copy f={f} a={CUT.sessions} b={CUT.taste} style={{ left: 120, top: 110, width: 1680, height: 200 }}>
        <Say f={f} size={92} style={{ left: 0, top: 0, width: 1680 }} words={[{ t: "Every", at: 0 }, { t: "listening", at: 0 }, { t: "session,", at: 0 }, { t: "\n", at: 0 }, { t: "play", at: w("playByPlay") }, { t: "by", at: 0 }, { t: "play.", at: 0 }]} />
      </Copy>

      <Copy f={f} a={CUT.taste} b={CUT.live} style={col}>
        <div style={headStyle(92)}>
          Watch your
          <br />
          taste move.
        </div>
        <Say f={f} size={46} style={{ ...subLine, whiteSpace: "nowrap" }} words={[{ t: "Week", at: w("weekByWeek") }, { t: "by", at: 0 }, { t: "week,", at: 0 }, { t: "year", at: 0 }, { t: "by", at: 0 }, { t: "year.", at: 0 }]} />
      </Copy>

      <Copy f={f} a={CUT.live} b={CUT.devices} style={{ left: 120, top: 110, width: 1680, height: 180 }}>
        <Say f={f} size={80} style={{ left: 0, top: 0, width: 1680 }} words={[{ t: "Every", at: 0 }, { t: "play", at: 0 }, { t: "as", at: 0 }, { t: "it", at: 0 }, { t: "happens.", at: 0 }, { t: "\n", at: 0 }, { t: "And", at: w("and") }, { t: "everything", at: 0 }, { t: "before.", at: 0 }]} />
      </Copy>

      <Copy f={f} a={CUT.devices} b={out} style={{ left: 120, top: 84, whiteSpace: "nowrap", ...headStyle(80) }}>
        On your desktop.{" "}
        <span style={{ opacity: r(f, pocket - 1, 10) }}>In your pocket.</span>
        <br />
        <span style={{ opacity: r(f, agent, 10) }}>
          Or just ask your <span style={{ color: C.lamp }}>AI agent.</span>
        </span>
      </Copy>
      <Copy f={f} a={CUT.devices} b={out} style={{ ...way, left: RECT.window.x, width: RECT.window.w }}>
        Blazing-fast web app
      </Copy>
      {f >= pocket - 8 && f < out ? (
        <>
          <Phone screen="overview" w={PHONE.w} style={{ left: mix(PHONE.x - 150, PHONE.x, ph), top: mix(PHONE.y + 110, PHONE.y, ph), opacity: Math.min(1, ph * 2) * (1 - phOut), transform: `translateY(${(-phOut * 20).toFixed(1)}px) rotate(${mix(-6, 0, ph).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 40px 90px rgba(0,0,0,.6)` }} />
          <div style={{ ...way, position: "absolute", left: PHONE.x - 40, width: PHONE.w + 80, opacity: r(f, pocket + 4, 8) * (1 - phOut) }}>Native iOS app</div>
        </>
      ) : null}
      <Copy f={f} a={agent - 1} b={out} style={{ ...surfaceStyle(AGENT), padding: 26, boxSizing: "border-box" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 14, height: 14, borderRadius: 7, background: C.lamp }} />
          <span style={headStyle(28)}>Your AI agent</span>
        </div>
        {/* The exchange arrives in order: the question, the tool it calls, the answer. */}
        <div style={{ marginTop: 34, marginLeft: 34, padding: "16px 20px", borderRadius: "20px 20px 6px 20px", background: "#23252b", fontFamily: FONT, fontSize: 25, fontWeight: 560, lineHeight: 1.3, color: C.paper, opacity: r(f, agent + 4, 7) }}>Who did I play most this summer?</div>
        <div style={{ marginTop: 30, display: "flex", alignItems: "center", gap: 10, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 19, color: MUTE, opacity: r(f, agent + 10, 7) }}>
          <span style={{ width: 9, height: 9, borderRadius: 5, background: C.lamp }} />
          spotussi · get_top
        </div>
        <div style={{ marginTop: 26, opacity: r(f, agent + 16, 8) }}>
          {[
            { x: ARTISTS[0], n: 412 },
            { x: ARTISTS[2], n: 365 },
            { x: ARTISTS[4], n: 301 },
          ].map(({ x, n }, k) => (
            <div key={x.name} style={{ display: "flex", alignItems: "center", gap: 14, height: 68 }}>
              <span style={{ width: 20, ...headStyle(22), color: k ? MUTE : C.lamp }}>{k + 1}</span>
              <Art seed={x.seed} size={46} round />
              <span style={{ flex: 1, fontFamily: FONT, fontSize: 24, fontWeight: 620, color: C.paper, whiteSpace: "nowrap" }}>{x.name}</span>
              <span style={{ fontFamily: FONT, fontSize: 24, fontWeight: 680, color: C.paper, fontVariantNumeric: "tabular-nums" }}>{n}</span>
            </div>
          ))}
        </div>
      </Copy>
      <Copy f={f} a={agent + 3} b={out} style={{ ...way, left: AGENT.x, width: AGENT.w }}>
        Any MCP client
      </Copy>
    </>
  );
};

// ── 10. Sign-off and call to action. The window folds into the mark and the meter lights on the
// music's downbeat as she says the name; the repo follows; then "Self-host in one command" and
// the command, which types itself.
const CMD = "docker compose up -d";
const SignOff = ({ f }: { f: number }) => {
  const a = CUT.end;
  if (f < a - 4) return null;
  const host = w("selfHost");
  const repo = r(f, w("signOff") + 14, 10); // the repo follows the name
  const box = r(f, host - 3, 10);
  const typed = Math.max(0, Math.min(CMD.length, Math.floor(f - (host + 2))));
  const fade = r(f, FINAL + 6, DURATION - FINAL - 8, E.inOut);
  const centred: React.CSSProperties = { position: "absolute", left: 0, right: 0, textAlign: "center" };
  return (
    <AbsoluteFill style={{ opacity: 1 - fade }}>
      <Lockup x={RECT.mark.x} y={END_LOCK.y} size={END_LOCK.size} grille={r(f, a - 4, 5)} lamp={(i) => r(f, a - 1 + i * 0.8, 4)} word={r(f, a, 8)} wordDx={(1 - r(f, a, 10)) * -24} />
      <div style={{ ...centred, top: 452, ...headStyle(54), fontStretch: "112%", fontWeight: 660, letterSpacing: "-0.015em", opacity: repo, transform: `translateY(${((1 - repo) * 20).toFixed(1)}px)` }}>github.com/jasoncronje/spotussi</div>
      {/* The call to action: the label, then the install command types into a terminal pill. */}
      <div style={{ ...centred, top: 636, fontFamily: FONT, fontSize: 34, fontWeight: 560, color: C.mist, opacity: box }}>Self-host in one command</div>
      <div style={{ position: "absolute", left: 960 - 420, top: 696, width: 840, height: 100, boxSizing: "border-box", borderRadius: 50, background: "#101114", boxShadow: `0 0 0 2px ${EDGE}`, opacity: box, display: "flex", alignItems: "center", padding: "0 44px", gap: 22, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 38, color: C.paper }}>
        <span style={{ color: C.lamp }}>$</span>
        <span>{CMD.slice(0, typed)}</span>
        <span style={{ width: 18, height: 40, background: C.lamp, opacity: typed < CMD.length ? 1 : 0, marginLeft: -14 }} />
      </div>
      <div style={{ ...centred, top: 988, fontFamily: FONT, fontSize: 26, color: MUTE, opacity: repo }}>Not affiliated with Spotify. Spotify and Wrapped are trademarks of Spotify AB.</div>
    </AbsoluteFill>
  );
};

// ── Audio: the voice on top; the music 10 dB under it for as long as she speaks, then up to the
// voice's level for the last bars and the final hit. The music's own arc (filtered intro, fill,
// lift, ending) is in the file. MASTER lifts the mix to about −14 LUFS with peaks under −1 dBFS.
const MASTER = 1.17;
const DUCK = 0.316;
const Music = () => {
  const end = w("end");
  return <Audio src={staticFile("audio/bgm-v4.wav")} volume={(f) => MASTER * interpolate(f, [end, end + 18], [DUCK, 0.8], clamp)} />;
};

export const Launch = ({ bgm = true }: { bgm?: boolean }) => {
  const f = useCurrentFrame();
  return (
    <AbsoluteFill style={{ fontFamily: FONT, background: "#0a0b0d" }}>
      <Year f={f} />
      <Intro f={f} />
      <Surface f={f} />
      <Captions f={f} />
      <SignOff f={f} />
      <AbsoluteFill style={{ background: "#000", opacity: 1 - r(f, 0, 20, E.inOut) }} />
      {bgm ? <Music /> : null}
      <Sequence from={Math.round(TIMING.voOffset * FPS)}>
        <Audio src={staticFile("audio/vo-v4.wav")} volume={MASTER} />
      </Sequence>
    </AbsoluteFill>
  );
};
