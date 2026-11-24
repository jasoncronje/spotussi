// The spotussi launch film (draft 11), built as motion design. Every piece of UI is drawn in code in
// the app's own design language (graphite panels, Archivo, lamp meters), with invented artists and
// generated cover art. Voice-over and music only. Ends on the call to action.
//
// 33 s at 30 fps. All timing comes from timing.json, which scripts/audio.py writes next to the
// audio it builds: `words` are the frames at which the voice says each cue word, and every slide
// change sits on a beat of the music, a few frames before the slide's first word.
import type React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";
import { C, FONT, mulberry32 } from "./kit";
import { Phone, headStyle } from "./parts";
import TIMING from "./timing.json";

const FPS = 30;
export const DURATION = TIMING.duration;
/** The frame at which the voice says this cue word. */
const w = (k: keyof typeof TIMING.words) => Math.round(TIMING.words[k] * FPS);
/** The frame of the music's nth beat. */
const beat = (n: number) => Math.round((TIMING.beat.phase + n * TIMING.beat.period) * FPS);
/** Slide changes, each on a beat. */
const CUT = {
  lock: beat(TIMING.cutBeats.lock),
  top: beat(TIMING.cutBeats.top),
  artist: beat(TIMING.cutBeats.artist),
  sessions: beat(TIMING.cutBeats.sessions),
  taste: beat(TIMING.cutBeats.taste),
  live: beat(TIMING.cutBeats.live),
  devices: beat(TIMING.cutBeats.devices),
  end: beat(TIMING.cutBeats.end),
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
/** The mark: which lamps of the 4×4 grille are lit, as [column, row]. */
const LIT: [number, number][] = [[0, 2], [0, 3], [1, 0], [1, 1], [1, 2], [1, 3], [2, 1], [2, 2], [2, 3], [3, 3]];
const Panel = ({ children, style }: { children?: React.ReactNode; style?: React.CSSProperties }) => (
  <div style={{ position: "absolute", background: PANEL, borderRadius: 22, boxShadow: `0 0 0 1.5px ${EDGE}`, overflow: "hidden", ...style }}>{children}</div>
);
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

/** A scene's container: a plain fade in and out. The scene's one motion lives inside it. */
const Stage = ({ f, a, b, children, inD = 6, outD = 4 }: { f: number; a: number; b: number; children: React.ReactNode; inD?: number; outD?: number }) => {
  if (f < a || f >= b) return null;
  return <AbsoluteFill style={{ opacity: r(f, a, inD, E.out) * (1 - r(f, b - outD, outD, E.in)) }}>{children}</AbsoluteFill>;
};

// ── Background: flat graphite.
const Atmos = (_: { f: number }) => <AbsoluteFill style={{ background: "#0a0b0d" }} />;

// ── 1–2. The tear-off calendar. Wrapped: the year's pages flip by blank, and only one page in
// December has anything on it. spotussi: the year flips by again and every page is full — that
// day's top song and time listened — faster and faster, then the year stacks into your server.
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
  const song = SONGS[(day * 7) % SONGS.length];
  const mins = Math.round(20 + DAYS[day] * 240);
  return (
    <div style={{ position: "absolute", inset: 0, borderRadius: 26, background: wrapped ? "#0f2a19" : "#17181c", boxShadow: `inset 0 0 0 1.5px ${wrapped ? "#1f6b3c" : EDGE}`, overflow: "hidden" }}>
      {/* Header band: month and weekday, like a desk calendar's red strip, in graphite. */}
      <div style={{ height: 96, background: wrapped ? C.lamp : "#23252b", display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 36px" }}>
        <span style={{ ...headStyle(40), color: wrapped ? "#06140b" : C.paper }}>{m}</span>
        <span style={{ fontFamily: FONT, fontSize: 24, fontWeight: 600, color: wrapped ? "#06140b" : MUTE }}>{WEEKDAYS[day % 7]}</span>
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

const Calendar = ({ f, pages, times, durs }: { f: number; pages: { day: number; kind: PageKind }[]; times: number[]; durs: number[] }) => {
  let k = 0;
  while (k < times.length && f >= times[k] + durs[k]) k++; // pages fully gone
  const cur = Math.min(k, pages.length - 1);
  const flipping = k < times.length && f >= times[k] ? (f - times[k]) / durs[k] : 0;
  const pFlip = Easing.in(Easing.quad)(Math.min(1, flipping));
  return (
    <div style={{ position: "absolute", left: CAL.x, top: CAL.y, width: CAL.w, height: CAL.h, perspective: 1600 }}>
      {/* The pad's thickness: a few page edges under the top page. */}
      {[3, 2, 1].map((i) => <div key={i} style={{ position: "absolute", left: i * 4, right: -i * 4, top: i * 7, bottom: -i * 7, borderRadius: 26, background: "#121316", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }} />)}
      {cur + 1 < pages.length ? <CalPage {...pages[cur + 1]} /> : null}
      {/* The top page folds back over the binding. It stays solid while it still covers the page
          under it and stays inside the frame, so two pages' type never show through each other. */}
      <div style={{ position: "absolute", inset: 0, transformOrigin: "50% 0%", transform: `translateY(${(-pFlip * 150).toFixed(1)}px) rotateX(${(-pFlip * 78).toFixed(2)}deg)`, opacity: pFlip >= 1 ? 0 : 1 - Math.max(0, pFlip - 0.8) / 0.2 }}>
        <CalPage {...pages[cur]} />
      </div>
      {/* Binding rings. */}
      {[0.28, 0.72].map((x) => <div key={x} style={{ position: "absolute", left: CAL.w * x - 14, top: -18, width: 28, height: 44, borderRadius: 14, background: "#2f3138", boxShadow: "inset 0 0 0 3px #3f424b" }} />)}
    </div>
  );
};

const Year = ({ f }: { f: number }) => {
  const b = CUT.top;
  if (f >= b) return null;
  const once = w("once"); // "once a year": land on the Wrapped page
  const lock = CUT.lock; // spotussi: the year again, every page full
  const server = w("on"); // "On your own server."
  const exit = r(f, b - 4, 4, E.in);

  // Act 1: blank pages Jan → Nov, landing on Dec 1, Wrapped.
  const blankDays = Array.from({ length: 8 }, (_, i) => Math.round((i / 8) * (WRAPPED_DAY - 30)));
  const act1 = [...blankDays.map((day) => ({ day, kind: "blank" as PageKind })), { day: WRAPPED_DAY, kind: "wrapped" as PageKind }];
  const s1 = schedule(16, once - 2, act1.length - 1, 1);
  // Act 2: the Wrapped page tears away; Jan 1 onwards, every page full, done before the pad moves.
  const fullDays = Array.from({ length: 16 }, (_, i) => Math.round((i / 15) * 364));
  const act2 = fullDays.map((day) => ({ day, kind: "full" as PageKind }));
  const s2 = schedule(lock + 22, server - 12, act2.length - 1, 0.8);
  const inAct2 = f >= lock;
  const tear = r(f, lock, 14, E.in); // the Wrapped page tears off and falls away, solid until it has left the pad
  const dayNow = inAct2 ? act2[Math.min(act2.length - 1, s2.times.filter((t) => f >= t).length)].day : 0;

  // Act 3, in strict turns: the copy clears, the pad shrinks up out of the way, the home server
  // rises in under it, the pad drops through the slot and the lamps light.
  const shrink = r(f, server - 8, 10, E.inOut);
  const device = r(f, server, 12);
  const drop = r(f, server + 8, 12, E.in);
  const lamps = r(f, server + 20, 6);
  const SLOT_Y = 540;

  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      {/* The pad. Once it drops, it is clipped at the server's top edge: it goes into the slot. */}
      <div style={{ position: "absolute", inset: 0, clipPath: drop > 0 ? `inset(0 0 ${1080 - SLOT_Y}px 0)` : undefined }}>
        <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, transform: `translateY(${(mix(0, -220, shrink) + drop * 600).toFixed(1)}px) scale(${mix(1, 0.5, shrink).toFixed(4)})`, transformOrigin: `${CAL.x + CAL.w / 2}px ${CAL.y + CAL.h / 2}px` }}>
          {!inAct2 ? (
            <Calendar f={f} pages={act1} times={s1.times} durs={s1.durs} />
          ) : (
            <>
              <Calendar f={f} pages={act2} times={s2.times} durs={s2.durs} />
              {tear < 1 ? (
                <div style={{ position: "absolute", left: CAL.x, top: CAL.y, width: CAL.w, height: CAL.h, transformOrigin: "100% 0%", transform: `translate(${(tear * 300).toFixed(1)}px, ${(tear * 700).toFixed(1)}px) rotate(${(tear * 28).toFixed(2)}deg)`, opacity: 1 - Math.max(0, tear - 0.75) / 0.25 }}>
                  <CalPage day={WRAPPED_DAY} kind="wrapped" />
                </div>
              ) : null}
            </>
          )}
        </div>
      </div>
      {/* The home server: one unit, its front panel is the mark. */}
      {f >= server ? (
        <div style={{ position: "absolute", left: 120, top: SLOT_Y, width: 1680, height: 300, borderRadius: 28, background: "#17181c", boxShadow: `0 0 0 1.5px ${EDGE}`, opacity: r(f, server, 6), transform: `translateY(${((1 - device) * 40).toFixed(1)}px)` }}>
          <div style={{ position: "absolute", left: CAL.x + CAL.w / 2 - 120 - 170, top: -2, width: 340, height: 12, borderRadius: 6, background: "#050506" }} />
          <svg width={180} height={180} style={{ position: "absolute", left: 70, top: 60 }}>
            {Array.from({ length: 16 }, (_, i) => {
              const col = Math.floor(i / 4);
              const row = i % 4;
              const pitch = 180 / 3.786;
              const rr = pitch * 0.393;
              const lit = LIT.some(([c, w2]) => c === col && w2 === row);
              return (
                <g key={i}>
                  <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr} fill={C.grille} />
                  {lit ? <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr} fill={C.lamp} opacity={lamps} /> : null}
                </g>
              );
            })}
          </svg>
          <div style={{ position: "absolute", left: 320, top: 86 }}>
            <div style={{ ...headStyle(64) }}>spotussi.local</div>
            <div style={{ fontFamily: FONT, fontSize: 30, color: MUTE, marginTop: 16 }}>Your history, on your hardware.</div>
          </div>
        </div>
      ) : null}
      {/* Copy, left column, in strict turns: each block is gone before the next arrives. */}
      <Say
        f={f}
        size={88}
        out={lock - 7}
        style={{ left: 120, top: 400, width: 1040 }}
        words={[
          { t: "Spotify", at: 0 },
          { t: "Wrapped", at: 0 },
          { t: "\n", at: 0 },
          { t: "comes", at: w("comes") },
          { t: "once", at: w("once") },
          { t: "a", at: 0 },
          { t: "year.", at: 0 },
        ]}
      />
      {f >= lock && f < server - 3 ? (
        <div style={{ position: "absolute", left: 120, top: 330, width: 1040, opacity: r(f, lock, 8) * (1 - r(f, server - 8, 5, E.in)) }}>
          <Img src={staticFile("logo/lockup-dark.svg")} style={{ height: 64 }} />
          <div style={{ ...headStyle(80), marginTop: 40 }}>
            Your whole history,
            <br />
            <span style={{ color: C.lamp, opacity: r(f, w("every") - 1, 8) }}>every single day.</span>
          </div>
          <div style={{ fontFamily: FONT, fontSize: 34, fontWeight: 600, color: MUTE, marginTop: 36, fontVariantNumeric: "tabular-nums", opacity: r(f, lock + 24, 8) }}>
            Day {dayNow + 1} of 365
          </div>
        </div>
      ) : null}
      <Say f={f} size={96} style={{ left: 120, top: 370 }} words={[{ t: "On", at: server }, { t: "your", at: 0 }, { t: "own", at: 0 }, { t: "server.", at: 0 }]} />
    </AbsoluteFill>
  );
};

/** A second, quieter line under a headline, for the sentence that follows it. */
const subLine: React.CSSProperties = { position: "relative", color: C.mist, fontStretch: "100%", fontWeight: 520, letterSpacing: "0" };

// ── 3. Your real top 10: the list starts on "This month"; on "Not just this month's" the switch
// slides to "All time" and the rows re-sort, bars and counts running to their lifetime values.
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
const TopRow = ({ x, rank, val, frac, lit = false, rankOpacity = 1 }: { x: (typeof ARTISTS)[number]; rank: number; val: number; frac: number; lit?: boolean; rankOpacity?: number }) => (
  <div style={{ height: TOP_ROW, padding: "0 12px", display: "flex", alignItems: "center", gap: 20 }}>
    <div style={{ width: 36, textAlign: "right", ...headStyle(26), color: lit ? C.lamp : MUTE, opacity: rankOpacity }}>{rank}</div>
    <Art seed={x.seed} size={46} round />
    <div style={{ width: 290, fontFamily: FONT, fontSize: 24, fontWeight: 620, color: C.paper, whiteSpace: "nowrap" }}>{x.name}</div>
    <div style={{ flex: 1, height: 8, borderRadius: 4, background: "#23252b" }}>
      <div style={{ width: `${(frac * 100).toFixed(2)}%`, height: 8, borderRadius: 4, background: C.lamp }} />
    </div>
    <div style={{ width: 110, textAlign: "right", fontFamily: FONT, fontSize: 24, fontWeight: 680, color: C.paper, fontVariantNumeric: "tabular-nums" }}>{fmt(val)}</div>
  </div>
);
const Top = ({ f }: { f: number }) => {
  const a = CUT.top;
  const b = CUT.artist;
  const sw = w("notJust") - 3; // the switch starts a breath before "Not just this month's"
  const pill = r(f, sw, 14, E.inOut);
  const byMonth = [...ARTISTS].sort((x, y) => y.month - x.month);
  const byAll = [...ARTISTS].sort((x, y) => y.all - x.all);
  const maxM = byMonth[0].month;
  const maxA = byAll[0].all;
  // One lead: the all-time #1 rises to the top as a solid strip, its count running up. Nothing
  // else travels. The rows in its path clear first; the rest of the month's rows dim and hold
  // still; once the lead has passed, each slot takes its all-time artist, top to bottom, old out
  // before new in. No row ever crosses another.
  const lead = byAll[0];
  const leadFrom = byMonth.indexOf(lead);
  const rise = r(f, sw + 2, 14, E.inOut);
  const swap = (i: number) => sw + 14 + i; // the frame slot i takes its all-time artist
  const slot = (i: number): React.CSSProperties => ({ position: "absolute", left: 28, right: 28, top: 108 + i * TOP_ROW });
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 880, top: 120, width: 920, height: 840 }}>
        <div style={{ position: "absolute", left: 40, top: 36, ...headStyle(40) }}>Top artists</div>
        {/* The range switch slides from This month to All time. */}
        <div style={{ position: "absolute", right: 40, top: 26, width: 400, height: 60, borderRadius: 30, background: "#0d0e11", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
          <div style={{ position: "absolute", top: 6, left: mix(6, 200, pill), width: 194, height: 48, borderRadius: 24, background: "#2a2c33" }} />
          {["This month", "All time"].map((l, i) => (
            <div key={l} style={{ position: "absolute", top: 0, left: i ? 200 : 6, width: 194, lineHeight: "60px", textAlign: "center", fontFamily: FONT, fontSize: 24, fontWeight: 640, color: (i ? pill : 1 - pill) > 0.5 ? C.paper : MUTE }}>{l}</div>
          ))}
        </div>
        {byMonth.map((x, i) =>
          x === lead || f >= swap(i) ? null : (
            <div key={`m-${x.name}`} style={{ ...slot(i), opacity: i < leadFrom ? 1 - r(f, sw, 5, E.in) : mix(1, 0.3, r(f, sw, 4)) * (1 - r(f, swap(i) - 3, 3, E.in)) }}>
              <TopRow x={x} rank={i + 1} val={x.month} frac={x.month / maxM} />
            </div>
          ),
        )}
        {byAll.map((x, i) =>
          i === 0 || f < swap(i) ? null : (
            <div key={`a-${x.name}`} style={{ ...slot(i), opacity: r(f, swap(i), 6) }}>
              <TopRow x={x} rank={i + 1} val={x.all} frac={x.all / maxA} />
            </div>
          ),
        )}
        <div style={{ ...slot(mix(leadFrom, 0, rise)), background: PANEL, zIndex: 2 }}>
          <TopRow x={lead} rank={rise < 0.5 ? leadFrom + 1 : 1} rankOpacity={Math.abs(1 - 2 * rise)} lit={rise >= 0.5} val={mix(lead.month, lead.all, r(f, sw + 2, 22, E.out))} frac={mix(lead.month / maxM, 1, rise)} />
        </div>
      </Panel>
      <div style={{ position: "absolute", left: 120, top: 400, width: 740, display: "flex", flexDirection: "column", gap: 30 }}>
        <Say f={f} size={92} style={{ position: "relative" }} words={[{ t: "Your", at: 0 }, { t: "real", at: 0, lit: true }, { t: "\n", at: 0 }, { t: "top", at: 0 }, { t: "10.", at: 0 }]} />
        <Say f={f} size={46} style={subLine} words={[{ t: "Not", at: w("notJust") }, { t: "just", at: 0 }, { t: "this", at: 0 }, { t: "month's.", at: 0 }]} />
      </div>
    </Stage>
  );
};

// ── 4. Your #1 artist: who it is, how many plays all time, and since when.
const YEARLY = [0.18, 0.22, 0.3, 0.26, 0.41, 0.48, 0.55, 0.72, 0.95, 0.8, 0.67, 0.74, 0.61, 0.7, 0.83, 0.58];
const ArtistShot = ({ f }: { f: number }) => {
  const a = CUT.artist;
  const b = CUT.sessions;
  // The count runs from the moment the slide arrives and settles as she says "twelve thousand
  // plays", so the panel never sits on a zero.
  const count = r(f, a, w("twelve") + 14 - a, E.out);
  const first = r(f, w("since") - 1, 10);
  return (
    <Stage f={f} a={a} b={b}>
      <Say f={f} size={110} style={{ left: 120, top: 120, width: 1680 }} words={[{ t: "Your", at: 0 }, { t: "#1", at: 0, lit: true }, { t: "artist.", at: 0 }]} />
      {/* Who: the artist, large, on the left. */}
      <div style={{ position: "absolute", left: 120, top: 360 }}>
        <Art seed={1} size={330} round />
        <div style={{ ...headStyle(76), marginTop: 40 }}>Nova Reyes</div>
        <div style={{ fontFamily: FONT, fontSize: 30, color: MUTE, marginTop: 10 }}>Synthpop</div>
      </div>
      {/* The proof: plays all time, the first play, and every year since. */}
      <Panel style={{ left: 880, top: 360, width: 920, height: 600 }}>
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
      </Panel>
    </Stage>
  );
};

// ── 5. Sessions: three days, two sittings each, every play a cover. One playhead runs across and
// the plays light as it passes.
const SESSIONS = [
  { day: "Mon", blocks: [{ x: 0, n: 6, span: "07:48 – 08:09" }, { x: 760, n: 9, span: "18:20 – 18:51" }] },
  { day: "Wed", blocks: [{ x: 300, n: 8, span: "12:05 – 12:33" }, { x: 1040, n: 5, span: "21:40 – 21:57" }] },
  { day: "Sat", blocks: [{ x: 120, n: 9, span: "10:30 – 11:02" }, { x: 900, n: 7, span: "16:15 – 16:39" }] },
];
const Sessions = ({ f }: { f: number }) => {
  const a = CUT.sessions;
  const b = CUT.taste;
  const X0 = 150; // where the lanes start inside the panel
  const W = 1490;
  const TILE = 64;
  const headX = X0 + r(f, a + 8, b - a - 22, (t) => t) * W;
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 120, top: 360, width: 1680, height: 600 }}>
        <div style={{ position: "absolute", left: 40, top: 30, ...headStyle(36) }}>Sessions</div>
        {SESSIONS.map((lane, li) => (
          <div key={lane.day} style={{ position: "absolute", left: 0, right: 0, top: 112 + li * 158 }}>
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
      </Panel>
      <Say f={f} size={92} style={{ left: 120, top: 110, width: 1680 }} words={[{ t: "Every", at: 0 }, { t: "listening", at: 0 }, { t: "session,", at: 0 }, { t: "\n", at: 0 }, { t: "play", at: w("playByPlay") }, { t: "by", at: 0 }, { t: "play.", at: 0 }]} />
    </Stage>
  );
};

// ── 6. Taste: stacked weekly bars whose genre bands shift as the weeks run; one genre grows.
const GENRES = [
  { name: "Indie", col: "#5b5f68" },
  { name: "House", col: "#3a3d46" },
  { name: "Synthpop", col: C.lamp },
  { name: "Hip hop", col: "#80848e" },
];
const Taste = ({ f }: { f: number }) => {
  const a = CUT.taste;
  const b = CUT.live;
  const N = 12;
  const H = 520;
  const shift = r(f, a + 6, b - a - 16, E.inOut); // starts once the headline is up
  const synAt = (week: number) => 0.12 + 0.55 * Math.min(1, Math.max(0, (week / (N - 1)) * shift * 1.25));
  const lastSyn = synAt(N - 1);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 880, top: 150, width: 920, height: 780 }}>
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
            <div key={week} style={{ position: "absolute", left: 40 + week * 58, bottom: 60, width: 42, height: H, borderRadius: 10, overflow: "hidden", display: "flex", flexDirection: "column-reverse" }}>
              {parts.map((p, k) => <div key={k} style={{ height: `${(p * 100).toFixed(2)}%`, background: GENRES[k].col, borderTop: "2px solid #15161a" }} />)}
            </div>
          );
        })}
        {/* The growing band names itself: the label rides the middle of the last week's green band. */}
        <div style={{ position: "absolute", left: 40 + (N - 1) * 58 + 42 + 18, bottom: 60 + ((1 - lastSyn) * 0.75 + lastSyn / 2) * H - 17, fontFamily: FONT, fontSize: 28, fontWeight: 680, lineHeight: "34px", color: C.lamp }}>Synthpop</div>
      </Panel>
      <div style={{ position: "absolute", left: 120, top: 400, width: 740, display: "flex", flexDirection: "column", gap: 30 }}>
        <Say f={f} size={92} style={{ position: "relative" }} words={[{ t: "Watch", at: 0 }, { t: "your", at: 0 }, { t: "\n", at: 0 }, { t: "taste", at: 0 }, { t: "move.", at: 0 }]} />
        <Say f={f} size={46} style={subLine} words={[{ t: "Week", at: w("week") }, { t: "on", at: 0 }, { t: "week.", at: 0 }]} />
      </div>
    </Stage>
  );
};

// ── 7. Live and imported, one after the other: a new play opens a row at the top of Recently
// played ("as it happens"); then the export imports and the whole past counts up ("everything before").
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
const LiveImport = ({ f }: { f: number }) => {
  const a = CUT.live;
  const b = CUT.devices;
  const drop = r(f, w("asItHappens") - 2, 14, E.soft);
  const imp = w("imports");
  const fill = r(f, imp, 28, E.inOut);
  const shown = r(f, imp, 6);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 120, top: 330, width: 820, height: 640 }}>
        <div style={{ position: "absolute", left: 36, top: 30, display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 14, height: 14, borderRadius: 7, background: C.lamp }} />
          <span style={{ ...headStyle(32) }}>Recently played</span>
        </div>
        {/* The new play opens its own row: the list moves down exactly as far as the row has grown,
            so it never sits on top of another row. */}
        <div style={{ position: "absolute", left: 36, right: 36, top: 100, height: 80 * drop, borderRadius: 14, overflow: "hidden", background: "#162a21", opacity: drop }}>
          <PlayRow t="Neon Harbor" a="The Midnight Arcade" s={21} when="now" lit />
        </div>
        {RECENT.map((x, i) => (
          <div key={x.t} style={{ position: "absolute", left: 36, right: 36, top: 100 + (i + drop) * 92 }}>
            <PlayRow t={x.t} a={x.a} s={x.s} when={`${(i + 1) * 3} min ago`} />
          </div>
        ))}
      </Panel>
      {/* The export arrives and the past streams in. */}
      <Panel style={{ left: 980, top: 330, width: 820, height: 640 }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(32) }}>Import your history</div>
        <div style={{ position: "absolute", left: 40, top: 110, right: 40, height: 140, borderRadius: 18, boxShadow: `inset 0 0 0 2px ${EDGE}`, display: "flex", alignItems: "center", gap: 22, padding: "0 28px" }}>
          <div style={{ width: 74, height: 90, borderRadius: 10, background: "#22242a", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, display: "flex", alignItems: "flex-end", justifyContent: "center", paddingBottom: 10, fontFamily: FONT, fontSize: 16, fontWeight: 700, color: MUTE }}>ZIP</div>
          <div>
            <div style={{ fontFamily: FONT, fontSize: 24, fontWeight: 640, color: C.paper }}>Extended streaming history</div>
            <div style={{ fontFamily: FONT, fontSize: 19, color: MUTE }}>from Spotify's data export</div>
          </div>
        </div>
        <div style={{ position: "absolute", left: 40, right: 40, top: 300, height: 12, borderRadius: 6, background: "#23252b" }}>
          <div style={{ width: `${(fill * 100).toFixed(2)}%`, height: 12, borderRadius: 6, background: C.lamp }} />
        </div>
        {/* Nothing is shown until the import starts: no zero waiting on screen. */}
        <div style={{ position: "absolute", left: 40, top: 340, ...headStyle(76), fontVariantNumeric: "tabular-nums", opacity: shown }}>
          {fmt(184213 * fill)}
          <span style={{ ...headStyle(30), color: MUTE, marginLeft: 18 }}>plays since 2011</span>
        </div>
        {YEARS.map((y, i) => {
          const hh = 40 + 90 * (0.35 + (0.65 * ((i * 37) % 11)) / 10);
          const on = Math.min(1, Math.max(0, fill * YEARS.length - i));
          return (
            <div key={y} style={{ position: "absolute", left: 40 + i * 46, bottom: 24, width: 32 }}>
              <div style={{ position: "relative", height: 130 }}>
                <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: hh, borderRadius: "6px 6px 0 0", background: "#1d1f24", overflow: "hidden" }}>
                  <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: hh * on, background: C.lamp }} />
                </div>
              </div>
              <div style={{ height: 36, marginLeft: -14, width: 60, paddingTop: 10, textAlign: "center", fontFamily: FONT, fontSize: 18, color: MUTE }}>{i % 5 === 0 ? y : ""}</div>
            </div>
          );
        })}
      </Panel>
      <Say f={f} size={80} style={{ left: 120, top: 110, width: 1680 }} words={[{ t: "Every", at: 0 }, { t: "play", at: 0 }, { t: "as", at: 0 }, { t: "it", at: 0 }, { t: "happens.", at: 0 }, { t: "\n", at: 0 }, { t: "And", at: w("and") }, { t: "everything", at: 0 }, { t: "before.", at: 0 }]} />
    </Stage>
  );
};

// ── 8. Desktop first, then pocket: the dashboard in its browser window holds the frame; the phone
// slides in beside it on "In your pocket".
const Devices = ({ f }: { f: number }) => {
  const a = CUT.devices;
  const b = CUT.end;
  const pocket = w("pocket");
  const ph = r(f, pocket - 8, 18, E.land);
  return (
    <Stage f={f} a={a} b={b}>
      <div style={{ position: "absolute", left: 120, top: 290, width: 1240, height: 740, borderRadius: 18, background: "#101114", boxShadow: `0 0 0 1.5px ${EDGE}`, overflow: "hidden" }}>
        <div style={{ height: 42, background: "#17181c", display: "flex", alignItems: "center", gap: 8, paddingLeft: 18 }}>
          {[0, 1, 2].map((i) => <span key={i} style={{ width: 12, height: 12, borderRadius: 6, background: C.grille }} />)}
          <span style={{ marginLeft: 420, padding: "4px 60px", borderRadius: 7, background: "#0b0c0f", color: MUTE, fontFamily: FONT, fontSize: 14 }}>spotussi.local</span>
        </div>
        <div style={{ position: "absolute", left: 30, top: 70, right: 30, display: "flex", gap: 20 }}>
          {[
            { l: "Time listened", n: 136, u: "h" },
            { l: "Plays", n: 2341 },
            { l: "Artists", n: 568 },
            { l: "Tracks", n: 1923 },
          ].map((x, i) => (
            <div key={x.l} style={{ flex: 1, height: 140, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: "18px 20px" }}>
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
        <div style={{ position: "absolute", left: 810, top: 240, width: 370, height: 480, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: 20 }}>
          <Label>Top artists</Label>
          {ARTISTS.slice(0, 5).map((x) => (
            <div key={x.name} style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 20 }}>
              <Art seed={x.seed} size={44} round />
              <span style={{ fontFamily: FONT, fontSize: 20, fontWeight: 620, color: C.paper }}>{x.name}</span>
            </div>
          ))}
        </div>
      </div>
      <Phone screen="overview" w={330} style={{ left: mix(1200, 1470, ph), top: mix(420, 260, ph), opacity: Math.min(1, ph * 2), transform: `rotate(${mix(-6, -2, ph).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 60px 120px rgba(0,0,0,.7)` }} />
      <div style={{ position: "absolute", left: 120, top: 100, whiteSpace: "nowrap", ...headStyle(80) }}>
        On your desktop.{" "}
        <span style={{ opacity: r(f, pocket - 1, 10) }}>
          In your <span style={{ color: C.lamp }}>pocket.</span>
        </span>
      </div>
    </Stage>
  );
};

// ── 9. Sign-off and call to action, one line of voice at a time: the lockup lights on the music's
// downbeat as she says the name; "Free and open source" brings the repo; "Self-host in one
// command" brings the command, which types itself.
const CMD = "docker compose up -d";
const SignOff = ({ f }: { f: number }) => {
  const a = CUT.end;
  if (f < a) return null;
  const size = 150;
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  const lockW = size * (1606.1 / 1157 + 4855 / 1157);
  const lampsOn = r(f, a + 1, 8);
  const free = r(f, w("free") - 1, 10);
  const repo = r(f, w("free") + 7, 10);
  const host = w("selfHost");
  const box = r(f, host - 3, 10);
  const typed = Math.max(0, Math.min(CMD.length, Math.floor(f - (host + 2))));
  const fade = r(f, FINAL + 6, DURATION - FINAL - 8, E.inOut);
  const centred: React.CSSProperties = { position: "absolute", left: 0, right: 0, textAlign: "center" };
  return (
    <AbsoluteFill style={{ opacity: r(f, a, 6) * (1 - fade) }}>
      <div style={{ position: "absolute", left: 960 - lockW / 2, top: 196, width: lockW, height: size }}>
        <svg width={size} height={size} style={{ position: "absolute", overflow: "visible" }}>
          {Array.from({ length: 16 }, (_, i) => {
            const col = Math.floor(i / 4);
            const row = i % 4;
            const lit = LIT.some(([c, w2]) => c === col && w2 === row);
            return (
              <g key={i}>
                <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr} fill={C.grille} />
                {lit ? <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr * lampsOn} fill={C.lamp} /> : null}
              </g>
            );
          })}
        </svg>
        <Img src={staticFile("logo/word.svg")} style={{ position: "absolute", left: size * (1606.1 / 1157), top: 0, width: size * (4855 / 1157), height: size }} />
      </div>
      <div style={{ ...centred, top: 398, ...headStyle(64), fontStretch: "112%", fontWeight: 660, letterSpacing: "-0.02em", opacity: free }}>
        Free and <span style={{ color: C.lamp }}>open source</span>
      </div>
      <div style={{ ...centred, top: 494, ...headStyle(46), fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.01em", opacity: repo }}>github.com/jasoncronje/spotussi</div>
      {/* The call to action: the label, then the install command types into a terminal pill. */}
      <div style={{ ...centred, top: 646, fontFamily: FONT, fontSize: 34, fontWeight: 560, color: C.mist, opacity: box }}>Self-host in one command</div>
      <div style={{ position: "absolute", left: 960 - 420, top: 706, width: 840, height: 100, borderRadius: 50, background: "#101114", boxShadow: `0 0 0 2px ${EDGE}`, opacity: box, display: "flex", alignItems: "center", padding: "0 44px", gap: 22, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 38, color: C.paper }}>
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
const MASTER = 1.23;
const DUCK = 0.316;
const Music = () => {
  const end = w("end");
  return <Audio src={staticFile("audio/bgm-v4.wav")} volume={(f) => MASTER * interpolate(f, [end, end + 18], [DUCK, 0.8], clamp)} />;
};

export const Launch = ({ bgm = true }: { bgm?: boolean }) => {
  const f = useCurrentFrame();
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Atmos f={f} />
      <Year f={f} />
      <Top f={f} />
      <ArtistShot f={f} />
      <Sessions f={f} />
      <Taste f={f} />
      <LiveImport f={f} />
      <Devices f={f} />
      <SignOff f={f} />
      <AbsoluteFill style={{ background: "#000", opacity: 1 - r(f, 0, 20, E.inOut) }} />
      {bgm ? <Music /> : null}
      <Sequence from={Math.round(TIMING.voOffset * FPS)}>
        <Audio src={staticFile("audio/vo-v2.wav")} volume={MASTER} />
      </Sequence>
    </AbsoluteFill>
  );
};
