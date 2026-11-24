// Draft 4: the spotussi launch promo, built as motion design. Every piece of UI is drawn in
// code in the app's own design language (graphite panels, Archivo, lamp meters), with invented
// artists and generated cover art, so every number counts, every row moves and every chart grows.
// No sound effects: voice-over and music only. Ends on the call to action.
//
// 31 s at 30 fps. Voice (vo.wav, ElevenLabs "Laurel") starts at 0.3 s; scenes are cued to its words.
import type React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";
import { C, FONT, mulberry32 } from "./kit";
import { Phone, headStyle } from "./parts";

export const DURATION = 930;
const VO = 0.3;
const v = (s: number) => Math.round((s + VO) * 30); // voice time → frame
const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
const E = {
  out: Easing.bezier(0.16, 1, 0.3, 1),
  soft: Easing.bezier(0.22, 1, 0.36, 1),
  inOut: Easing.bezier(0.65, 0, 0.35, 1),
  in: Easing.bezier(0.7, 0, 0.84, 0),
  pop: Easing.bezier(0.34, 1.45, 0.5, 1),
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
const Panel = ({ children, style }: { children?: React.ReactNode; style?: React.CSSProperties }) => (
  <div style={{ position: "absolute", background: PANEL, borderRadius: 22, boxShadow: `0 0 0 1.5px ${EDGE}, 0 40px 110px rgba(0,0,0,.55)`, overflow: "hidden", ...style }}>{children}</div>
);
const Label = ({ children, style }: { children: React.ReactNode; style?: React.CSSProperties }) => (
  <div style={{ fontFamily: FONT, fontSize: 18, fontWeight: 560, color: MUTE, letterSpacing: "0.01em", ...style }}>{children}</div>
);

type Wd = { t: string; at: number; lit?: boolean };
/** Words rise out of blur at their voice frames; the whole line clears with `out`. */
const Say = ({ words, f, size, out, style, align = "left" }: { words: Wd[]; f: number; size: number; out?: number; style?: React.CSSProperties; align?: "left" | "center" }) => {
  const gone = out !== undefined ? r(f, out, 7, E.in) : 0;
  if (gone >= 1) return null;
  return (
    <div style={{ position: "absolute", display: "flex", flexWrap: "wrap", justifyContent: align === "center" ? "center" : "flex-start", columnGap: "0.25em", ...headStyle(size), opacity: 1 - gone, filter: gone > 0 ? `blur(${(gone * 12).toFixed(1)}px)` : undefined, transform: `translateY(${(-gone * 24).toFixed(1)}px)`, ...style }}>
      {words.map((w, i) => {
        const p = r(f, w.at - 3, 10);
        return (
          <span key={i} style={{ display: "inline-block", color: w.lit ? C.lamp : undefined, opacity: p, transform: `translateY(${((1 - p) * size * 0.35).toFixed(1)}px)`, filter: p > 0.995 ? undefined : `blur(${((1 - p) * 14).toFixed(1)}px)` }}>
            {w.t}
          </span>
        );
      })}
    </div>
  );
};

/** A scene's container: zooms in from blur, drifts, and zooms through on the way out. */
const Stage = ({ f, a, b, children, inD = 7, outD = 7 }: { f: number; a: number; b: number; children: React.ReactNode; inD?: number; outD?: number }) => {
  if (f < a - inD || f > b + 2) return null;
  const pin = r(f, a - inD, inD, E.out);
  const pout = r(f, b - outD, outD, E.in);
  const drift = r(f, a, b - a, (t) => t);
  const s = mix(0.92, 1, pin) * (1 + 0.03 * drift) * (1 + 0.18 * pout);
  return (
    <AbsoluteFill style={{ opacity: Math.min(pin * 1.4, 1) * (1 - pout), transform: `scale(${s.toFixed(4)})`, filter: pin < 0.99 || pout > 0.01 ? `blur(${((1 - pin) * 16 + pout * 18).toFixed(1)}px)` : undefined }}>
      {children}
    </AbsoluteFill>
  );
};

// ── Background: graphite, a slow green glow that breathes with the scenes.
const Atmos = ({ f }: { f: number }) => (
  <AbsoluteFill style={{ background: "#08090b" }}>
    <AbsoluteFill style={{ background: `radial-gradient(ellipse 60% 55% at ${50 + Math.sin(f / 60) * 8}% ${105 + Math.cos(f / 80) * 6}%, rgba(29,185,84,.17) 0%, rgba(29,185,84,0) 70%)` }} />
    <AbsoluteFill style={{ background: `radial-gradient(ellipse 45% 40% at ${15 + Math.cos(f / 70) * 6}% ${0 + Math.sin(f / 90) * 6}%, rgba(110,120,150,.12) 0%, rgba(110,120,150,0) 70%)` }} />
    <AbsoluteFill style={{ background: "radial-gradient(ellipse 90% 85% at 50% 50%, rgba(0,0,0,0) 55%, rgba(0,0,0,.6) 100%)" }} />
  </AbsoluteFill>
);

// ── 1–2. A year of lamps. Wrapped lights one day a year; spotussi lights every one of them,
// then the year becomes one row of a whole history.
const YEARS = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024, 2025, 2026];
const Year = ({ f }: { f: number }) => {
  const b = v(6.96);
  if (f > b + 2) return null;
  const wrapped = v(1.06); // "once a year": one lamp lights
  const sweep = v(4.38); // "every single day": they all light
  const back = v(5.58); // pull back to the whole history
  const COLS = 53;
  const P = 26;
  const pull = r(f, back - 4, 26, E.inOut);
  const gridW = COLS * P;
  const rnd = mulberry32(7);
  const heights = Array.from({ length: COLS * 7 }, () => 0.35 + rnd() * 0.65);
  const one = 51 * 7 + 4; // the Wrapped day, early December
  const exit = r(f, b - 8, 9, E.in);
  const s = mix(1, 0.2, pull);
  const lock = r(f, v(2.16) - 2, 12);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit, filter: exit > 0 ? `blur(${(exit * 16).toFixed(1)}px)` : undefined, transform: `scale(${(1 + exit * 0.2).toFixed(3)})` }}>
      <div style={{ position: "absolute", left: 960, top: mix(600, 600 + 0.2 * 7.5 * 7 * 26 * 1.25, pull), transform: `translate(-50%,-50%) scale(${s.toFixed(4)})` }}>
        {(pull > 0 ? YEARS : [2026]).map((y, yi) => {
          const isMain = y === 2026;
          const rowY = isMain ? 0 : (yi - YEARS.length + 1) * 7 * P * 1.25;
          const rowOn = isMain ? 1 : r(f, back + 2 + (YEARS.length - 1 - yi) * 1.2, 10);
          return (
            <div key={y} style={{ position: "absolute", left: -gridW / 2, top: rowY - (7 * P) / 2, width: gridW, height: 7 * P, opacity: rowOn }}>
              <div style={{ position: "absolute", left: -150, top: 7 * P * 0.32, ...headStyle(48), color: MUTE, opacity: pull }}>{y}</div>
              {Array.from({ length: COLS * 7 }, (_, i) => {
                const col = Math.floor(i / 7);
                const row = i % 7;
                const lit = isMain ? (i === one ? r(f, wrapped, 6) : r(f, sweep + col * 0.45 + row * 0.3, 5)) : 1;
                const hk = heights[(i + y * 31) % heights.length];
                return (
                  <span key={i} style={{ position: "absolute", left: col * P, top: row * P, width: P * 0.72, height: P * 0.72, borderRadius: "50%", background: C.grille }}>
                    <span style={{ position: "absolute", inset: 0, borderRadius: "50%", background: C.lamp, opacity: lit * hk, transform: `scale(${(i === one ? 1 + 0.5 * r(f, wrapped, 4) * (1 - r(f, wrapped + 6, 10)) : 1).toFixed(3)})` }} />
                  </span>
                );
              })}
            </div>
          );
        })}
      </div>
      {/* The one day Wrapped lights: a ring and a label call it out, then go as every day lights. */}
      {(() => {
        const call = r(f, wrapped, 10, E.pop) * (1 - r(f, sweep - 6, 8, E.in));
        const cx = 960 - (53 * 26) / 2 + 51 * 26 + 26 * 0.36;
        const cy = 600 - (7 * 26) / 2 + 4 * 26 + 26 * 0.36;
        return call > 0 ? (
          <>
            <div style={{ position: "absolute", left: cx - 34, top: cy - 34, width: 68, height: 68, borderRadius: "50%", boxShadow: `0 0 0 3px ${C.lamp}`, opacity: call, transform: `scale(${mix(2.4, 1, call).toFixed(3)})` }} />
            <div style={{ position: "absolute", left: cx - 200, top: cy + 60, width: 400, textAlign: "center", ...headStyle(34), color: C.lamp, opacity: call }}>Wrapped</div>
          </>
        ) : null;
      })()}
      {/* Copy: the hook is on screen from the first frame. */}
      <Say
        f={f}
        size={92}
        out={v(2.16) - 6}
        align="center"
        style={{ left: 0, right: 0, top: 150 }}
        words={[
          { t: "Spotify", at: -10 },
          { t: "Wrapped", at: -10 },
          { t: "comes", at: v(0.84) },
          { t: "once", at: v(1.06) },
          { t: "a", at: v(1.46) },
          { t: "year.", at: v(1.66) },
        ]}
      />
      {f >= v(2.16) - 4 ? (
        <div style={{ position: "absolute", left: 0, right: 0, top: 120, textAlign: "center", opacity: lock * (1 - r(f, back - 6, 8, E.in)), transform: `translateY(${((1 - lock) * 20).toFixed(1)}px)`, filter: lock < 0.99 ? `blur(${((1 - lock) * 12).toFixed(1)}px)` : undefined }}>
          <Img src={staticFile("logo/lockup-dark.svg")} style={{ height: 70 }} />
          <div style={{ ...headStyle(60), marginTop: 26, fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.02em" }}>
            Your whole history, <span style={{ color: C.lamp, opacity: r(f, v(4.38), 8) }}>every single day.</span>
          </div>
        </div>
      ) : null}
      <Say f={f} size={64} align="center" style={{ left: 0, right: 0, top: 120 }} words={[{ t: "On", at: v(5.58) }, { t: "your", at: v(5.92) }, { t: "own", at: v(6.1) }, { t: "server.", at: v(6.38) }]} />
    </AbsoluteFill>
  );
};

// ── 3. Your real top 10: the list starts on "This month", the switch slides to "All time" and
// the rows re-sort themselves, bars and counts animating to their lifetime values.
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
const rankBy = (k: "all" | "month") => {
  const order = [...ARTISTS].sort((x, y) => y[k] - x[k]).map((x) => x.name);
  return (name: string) => order.indexOf(name);
};
const Top = ({ f }: { f: number }) => {
  const a = v(6.96);
  const b = v(9.56);
  const sw = v(8.3); // "not just this month's"
  const t = r(f, sw, 18, E.inOut);
  const mRank = rankBy("month");
  const aRank = rankBy("all");
  const maxM = 142;
  const maxA = 12418;
  const ROW = 66;
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 760, top: 120, width: 1020, height: 840 }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(40) }}>Top artists</div>
        {/* The range switch: a glass pill that slides from This month to All time. */}
        <div style={{ position: "absolute", right: 40, top: 30, width: 330, height: 52, borderRadius: 26, background: "#0d0e11", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
          <div style={{ position: "absolute", top: 5, left: mix(5, 167, t), width: 158, height: 42, borderRadius: 21, background: "#2a2c33" }} />
          {["This month", "All time"].map((l, i) => (
            <div key={l} style={{ position: "absolute", top: 15, left: i ? 167 : 5, width: 158, textAlign: "center", fontFamily: FONT, fontSize: 19, fontWeight: 640, color: (i ? t : 1 - t) > 0.5 ? C.paper : MUTE }}>{l}</div>
          ))}
        </div>
        {ARTISTS.map((x, i) => {
          const pos = mix(mRank(x.name), aRank(x.name), Easing.bezier(0.5, 0, 0.2, 1)(Math.min(1, Math.max(0, (f - sw - Math.abs(aRank(x.name) - mRank(x.name)) * 0.6) / 18))));
          const enter = r(f, a - 8 + mRank(x.name) * 2, 14, E.out);
          const val = mix(x.month, x.all, t);
          const frac = mix(x.month / maxM, x.all / maxA, t);
          const isOne = aRank(x.name) === 0 && t > 0.6;
          return (
            <div key={x.name} style={{ position: "absolute", left: 40, right: 40, top: 112 + pos * ROW, height: ROW - 8, display: "flex", alignItems: "center", gap: 20, opacity: enter, transform: `translateX(${((1 - enter) * 80).toFixed(1)}px)` }}>
              <div style={{ width: 36, textAlign: "right", ...headStyle(26), color: isOne ? C.lamp : MUTE }}>{Math.round(pos) + 1}</div>
              <Art seed={x.seed} size={46} round />
              <div style={{ width: 290, fontFamily: FONT, fontSize: 24, fontWeight: 620, color: C.paper, whiteSpace: "nowrap" }}>{x.name}</div>
              <div style={{ flex: 1, height: 8, borderRadius: 4, background: "#23252b" }}>
                <div style={{ width: `${(frac * 100 * r(f, a - 4 + i, 22)).toFixed(2)}%`, height: 8, borderRadius: 4, background: C.lamp }} />
              </div>
              <div style={{ width: 110, textAlign: "right", fontFamily: FONT, fontSize: 24, fontWeight: 680, color: C.paper, fontVariantNumeric: "tabular-nums" }}>{fmt(val)}</div>
            </div>
          );
        })}
        <AbsoluteFill style={{ background: `linear-gradient(0deg, ${PANEL} 0%, rgba(21,22,26,0) 12%)` }} />
      </Panel>
      <Say f={f} size={104} out={sw - 4} style={{ left: 140, top: 380, width: 600 }} words={[{ t: "Your", at: v(6.96) }, { t: "real", at: v(7.28), lit: true }, { t: "top", at: v(7.64) }, { t: "10.", at: v(7.9) }]} />
      <Say f={f} size={92} style={{ left: 140, top: 380, width: 600 }} words={[{ t: "Not", at: v(8.3) }, { t: "just", at: v(8.68) }, { t: "this", at: v(8.88) }, { t: "month's.", at: v(9.08) }]} />
    </Stage>
  );
};

// ── 4. One artist, all-time: the count rolls to 12,418 and a timeline draws from 2011.
const ArtistShot = ({ f }: { f: number }) => {
  const a = v(9.56);
  const b = v(12.52);
  const count = r(f, a - 2, 32, E.out);
  const since = v(11.1);
  const line = r(f, since - 4, 26, E.inOut);
  const rnd = mulberry32(31);
  const yearly = YEARS.map((_, i) => 0.25 + 0.75 * Math.min(1, (0.3 + i / 12) * (0.7 + rnd() * 0.5)));
  return (
    <Stage f={f} a={a} b={b}>
      <div style={{ position: "absolute", left: 160, top: 150, display: "flex", alignItems: "center", gap: 48 }}>
        <div style={{ transform: `scale(${mix(0.6, 1, r(f, a - 6, 18, E.pop)).toFixed(3)}) rotate(${(f - a) * 0.15}deg)` }}>
          <Art seed={1} size={250} round />
        </div>
        <div>
          <Label style={{ fontSize: 26 }}>Artist · #1 all time</Label>
          <div style={{ ...headStyle(96), marginTop: 6 }}>Nova Reyes</div>
        </div>
      </div>
      <div style={{ position: "absolute", left: 160, top: 470, ...headStyle(250), fontVariantNumeric: "tabular-nums", letterSpacing: "-0.05em" }}>
        {fmt(12418 * count)}
        <span style={{ ...headStyle(70), color: MUTE, marginLeft: 30 }}>plays</span>
      </div>
      {/* Since 2011: an axis draws left to right, a bar rises for each year as the line passes it. */}
      <div style={{ position: "absolute", left: 170, top: 900, width: 1580, height: 2, background: EDGE, transform: `scaleX(${line.toFixed(4)})`, transformOrigin: "0 50%" }} />
      {YEARS.map((y, i) => {
        const x = 170 + (i / (YEARS.length - 1)) * 1580;
        const on = r(f, since - 4 + i * 1.6, 12, E.out);
        return (
          <div key={y} style={{ position: "absolute", left: x - 26, top: 900 - 150 * yearly[i] * on, width: 52, height: 150 * yearly[i] * on, borderRadius: "10px 10px 0 0", background: i === YEARS.length - 1 ? C.lamp : "#2f3138" }}>
            {i % 3 === 0 || i === YEARS.length - 1 ? <div style={{ position: "absolute", top: 150 * yearly[i] * on + 14, left: -10, width: 72, textAlign: "center", fontFamily: FONT, fontSize: 20, color: MUTE, opacity: on }}>{y}</div> : null}
          </div>
        );
      })}
      <Say f={f} size={76} style={{ right: 160, top: 210, width: 560 }} words={[{ t: "Since", at: v(11.1) }, { t: "2011.", at: v(11.74), lit: true }]} />
    </Stage>
  );
};

// ── 5. Sessions: lanes of days; each session block builds play by play while a playhead runs.
const Sessions = ({ f }: { f: number }) => {
  const a = v(12.52);
  const b = v(14.94);
  const rnd = mulberry32(55);
  const lanes = ["Mon", "Tue", "Wed", "Thu", "Fri"];
  const head = r(f, a - 6, b - a + 6, (t) => t);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 140, top: 330, width: 1640, height: 600 }}>
        <div style={{ position: "absolute", left: 40, top: 30, ...headStyle(36) }}>Sessions</div>
        {lanes.map((d, li) => {
          const blocks = Array.from({ length: 3 }, (_, k) => {
            const start = 0.05 + k * 0.3 + rnd() * 0.08;
            const len = 0.12 + rnd() * 0.12;
            return { start, len, plays: 5 + Math.floor(rnd() * 9), seed: li * 3 + k };
          });
          return (
            <div key={d} style={{ position: "absolute", left: 40, right: 40, top: 110 + li * 92, height: 72 }}>
              <div style={{ position: "absolute", left: 0, top: 22, fontFamily: FONT, fontSize: 22, color: MUTE }}>{d}</div>
              {blocks.map((bk, k) => {
                const at = a - 10 + li * 3 + k * 9;
                const grow = r(f, at, 16, E.out);
                const W = 1440;
                return (
                  <div key={k} style={{ position: "absolute", left: 90 + bk.start * W, top: 6, width: bk.len * W * grow, height: 60, borderRadius: 12, background: "#1d1f24", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, overflow: "hidden", display: "flex", alignItems: "center", gap: 6, padding: "0 8px" }}>
                    {Array.from({ length: bk.plays }, (_, p) => {
                      const on = r(f, at + 3 + p * 1.5, 6, E.pop);
                      return <Art key={p} seed={(bk.seed * 7 + p) % 40} size={40} style={{ opacity: on, transform: `scale(${(0.5 + 0.5 * on).toFixed(3)})` }} />;
                    })}
                  </div>
                );
              })}
            </div>
          );
        })}
        <div style={{ position: "absolute", top: 90, bottom: 30, left: 130 + head * 1440, width: 3, background: C.lamp, boxShadow: "0 0 18px rgba(29,185,84,.6)" }} />
      </Panel>
      <Say f={f} size={92} align="center" style={{ left: 0, right: 0, top: 110 }} words={[{ t: "Every", at: v(12.52) }, { t: "listening", at: v(12.98) }, { t: "session,", at: v(13.44) }, { t: "play", at: v(13.98) }, { t: "by", at: v(14.26) }, { t: "play.", at: v(14.5) }]} />
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
  const a = v(14.94);
  const b = v(17.22);
  const W = 12;
  const shift = r(f, a, b - a, E.inOut);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 820, top: 150, width: 960, height: 780 }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(36) }}>Genres over time</div>
        <div style={{ position: "absolute", left: 40, top: 92, display: "flex", gap: 22 }}>
          {GENRES.map((g) => (
            <span key={g.name} style={{ display: "flex", alignItems: "center", gap: 8, fontFamily: FONT, fontSize: 19, color: g.col === C.lamp ? C.paper : MUTE }}>
              <span style={{ width: 12, height: 12, borderRadius: 6, background: g.col }} />
              {g.name}
            </span>
          ))}
        </div>
        {Array.from({ length: W }, (_, w) => {
          const on = r(f, a - 8 + w * 1.4, 14, E.out);
          const syn = 0.12 + 0.55 * Math.min(1, Math.max(0, (w / (W - 1)) * shift * 1.25));
          const rest = 1 - syn;
          const parts = [rest * 0.42, rest * 0.33, syn, rest * 0.25];
          return (
            <div key={w} style={{ position: "absolute", left: 60 + w * 72, bottom: 60, width: 50, height: 520 * on, borderRadius: 10, overflow: "hidden", display: "flex", flexDirection: "column-reverse" }}>
              {parts.map((p, k) => {
                return <div key={k} style={{ height: `${(p * 100).toFixed(2)}%`, background: GENRES[k].col, borderTop: "2px solid #15161a" }} />;
              })}
            </div>
          );
        })}
      </Panel>
      <Say f={f} size={104} style={{ left: 140, top: 360, width: 620 }} words={[{ t: "Watch", at: v(14.94) }, { t: "your", at: v(15.38) }, { t: "taste", at: v(15.54) }, { t: "move.", at: v(15.74), lit: true }]} />
      <Say f={f} size={44} style={{ left: 144, top: 620, width: 620, color: MUTE, fontStretch: "100%", fontWeight: 520, letterSpacing: "0" }} words={[{ t: "Week", at: v(16.02) }, { t: "on", at: v(16.58) }, { t: "week.", at: v(16.8) }]} />
    </Stage>
  );
};

// ── 7. Live and imported: a new play drops into Recently played; then the export zip lands and
// the whole past streams in.
const RECENT = [
  { t: "Glass Hours", a: "Nova Reyes", s: 11 },
  { t: "Night Signal", a: "Kairo", s: 13 },
  { t: "Paper Moons", a: "Static Bloom", s: 15 },
  { t: "Low Tide", a: "June Avenue", s: 16 },
  { t: "Afterglow FM", a: "Lumen Drift", s: 17 },
];
const LiveImport = ({ f }: { f: number }) => {
  const a = v(17.22);
  const b = v(20.82);
  const drop = r(f, v(17.56), 14, E.pop);
  const imp = v(19.48);
  const zip = r(f, imp - 8, 16, E.out);
  const fill = r(f, imp + 6, 26, E.inOut);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 140, top: 230, width: 820, height: 640 }}>
        <div style={{ position: "absolute", left: 36, top: 30, display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 14, height: 14, borderRadius: 7, background: C.lamp, opacity: 0.5 + 0.5 * Math.abs(Math.sin(f / 6)) }} />
          <span style={{ ...headStyle(32) }}>Recently played</span>
        </div>
        {[{ t: "Neon Harbor", a: "The Midnight Arcade", s: 21 }, ...RECENT].map((x, i) => {
          const isNew = i === 0;
          const y = 100 + (isNew ? 0 : (i - 1 + drop) * 92);
          return (
            <div key={x.t} style={{ position: "absolute", left: 36, right: 36, top: y, height: 80, display: "flex", alignItems: "center", gap: 20, borderRadius: 14, background: isNew ? `rgba(29,185,84,${(0.12 * drop).toFixed(3)})` : "transparent", padding: "0 12px", opacity: isNew ? drop : 1, transform: isNew ? `translateY(${((1 - drop) * -40).toFixed(1)}px) scale(${mix(0.9, 1, drop).toFixed(3)})` : undefined }}>
              <Art seed={x.s} size={58} />
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: FONT, fontSize: 25, fontWeight: 640, color: C.paper }}>{x.t}</div>
                <div style={{ fontFamily: FONT, fontSize: 19, color: MUTE }}>{x.a}</div>
              </div>
              <div style={{ fontFamily: FONT, fontSize: 19, color: isNew ? C.lamp : DIM }}>{isNew ? "now" : `${i * 3} min ago`}</div>
            </div>
          );
        })}
      </Panel>
      {/* The export arrives and the past streams in. */}
      <Panel style={{ left: 1020, top: 300, width: 760, height: 500, opacity: zip, transform: `translateY(${((1 - zip) * 60).toFixed(1)}px)` }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(32) }}>Import your history</div>
        <div style={{ position: "absolute", left: 40, top: 110, right: 40, height: 140, borderRadius: 18, boxShadow: `inset 0 0 0 2px ${EDGE}`, display: "flex", alignItems: "center", gap: 22, padding: "0 28px" }}>
          <div style={{ width: 74, height: 90, borderRadius: 10, background: "#22242a", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, display: "flex", alignItems: "flex-end", justifyContent: "center", paddingBottom: 10, fontFamily: FONT, fontSize: 16, fontWeight: 700, color: MUTE, transform: `translateY(${((1 - r(f, imp - 4, 14, E.pop)) * -90).toFixed(1)}px)` }}>ZIP</div>
          <div>
            <div style={{ fontFamily: FONT, fontSize: 24, fontWeight: 640, color: C.paper }}>Extended streaming history</div>
            <div style={{ fontFamily: FONT, fontSize: 19, color: MUTE }}>from Spotify's data export</div>
          </div>
        </div>
        <div style={{ position: "absolute", left: 40, right: 40, top: 300, height: 12, borderRadius: 6, background: "#23252b" }}>
          <div style={{ width: `${(fill * 100).toFixed(2)}%`, height: 12, borderRadius: 6, background: C.lamp }} />
        </div>
        <div style={{ position: "absolute", left: 40, top: 340, ...headStyle(76), fontVariantNumeric: "tabular-nums" }}>
          {fmt(184213 * fill)}
          <span style={{ ...headStyle(30), color: MUTE, marginLeft: 18 }}>plays since 2011</span>
        </div>
      </Panel>
      <Say f={f} size={60} align="center" style={{ left: 0, right: 0, top: 110, fontStretch: "112%", fontWeight: 660, letterSpacing: "-0.02em" }} words={[{ t: "Every", at: v(17.76) }, { t: "play", at: v(18.12) }, { t: "as", at: v(18.48) }, { t: "it", at: v(18.72) }, { t: "happens.", at: v(18.88) }, { t: "And", at: v(19.2) }, { t: "everything", at: v(19.72), lit: true }, { t: "before.", at: v(20.2), lit: true }]} />
    </Stage>
  );
};

// ── 8. Desktop and pocket: a mini dashboard in a window, the phone slides out in front.
const Devices = ({ f }: { f: number }) => {
  const a = v(20.82);
  const b = v(23.08);
  const ph = r(f, v(22.2) - 6, 18, E.pop);
  const tilt = r(f, a, b - a, (t) => t);
  return (
    <Stage f={f} a={a} b={b}>
      <AbsoluteFill style={{ perspective: 1800 }}>
        <div style={{ position: "absolute", left: 170, top: 230, width: 1240, height: 760, borderRadius: 18, background: "#101114", boxShadow: `0 0 0 1.5px ${EDGE}, 0 60px 140px rgba(0,0,0,.6)`, transform: `rotateY(${mix(22, 12, tilt).toFixed(2)}deg)`, transformOrigin: "0% 50%", overflow: "hidden" }}>
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
                  {fmt(x.n * r(f, a + i * 3, 24))}
                  {x.u ?? ""}
                </div>
                <div style={{ display: "flex", gap: 5, marginTop: 10 }}>
                  {Array.from({ length: 14 }, (_, k) => <span key={k} style={{ width: 9, height: 9, borderRadius: 5, background: k < 6 + i * 2 && f > a + i * 3 + k ? C.lamp : C.grille }} />)}
                </div>
              </div>
            ))}
          </div>
          <div style={{ position: "absolute", left: 30, top: 240, width: 760, height: 480, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
            <Label style={{ position: "absolute", left: 20, top: 18 }}>Plays by day</Label>
            {Array.from({ length: 28 }, (_, i) => {
              const hgt = 80 + 280 * Math.abs(Math.sin(i * 1.7) * 0.6 + Math.sin(i * 0.4) * 0.4);
              const on = r(f, a + 4 + i * 0.6, 14);
              return <div key={i} style={{ position: "absolute", left: 24 + i * 26, bottom: 24, width: 16, height: hgt * on, borderRadius: 4, background: i === 18 ? C.lamp : "#2f3138" }} />;
            })}
          </div>
          <div style={{ position: "absolute", left: 810, top: 240, width: 370, height: 480, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: 20 }}>
            <Label>Top artists</Label>
            {ARTISTS.slice(0, 5).map((x, i) => (
              <div key={x.name} style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 20, opacity: r(f, a + 6 + i * 2, 10) }}>
                <Art seed={x.seed} size={44} round />
                <span style={{ fontFamily: FONT, fontSize: 20, fontWeight: 620, color: C.paper }}>{x.name}</span>
              </div>
            ))}
          </div>
        </div>
      </AbsoluteFill>
      <Phone screen="overview" w={330} style={{ left: mix(1150, 1420, ph), top: mix(420, 250, ph), opacity: Math.min(1, ph * 2), transform: `rotate(${mix(-6, -2, ph).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 60px 120px rgba(0,0,0,.7)` }} />
      <Say f={f} size={70} align="center" style={{ left: 0, right: 0, top: 80 }} words={[{ t: "On", at: v(20.82) }, { t: "your", at: v(21.22) }, { t: "desktop.", at: v(21.36) }, { t: "In", at: v(22.2) }, { t: "your", at: v(22.46) }, { t: "pocket.", at: v(22.6), lit: true }]} />
    </Stage>
  );
};

// ── 9. Sign-off and call to action: the lockup lights lamp by lamp, then the command types out.
const LIT: [number, number][] = [[0, 2], [0, 3], [1, 0], [1, 1], [1, 2], [1, 3], [2, 1], [2, 2], [2, 3], [3, 3]];
const CMD = "docker compose up -d";
const SignOff = ({ f }: { f: number }) => {
  const a = v(23.08);
  if (f < a - 8) return null;
  const size = 150;
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  const lockW = size * (1606.1 / 1157 + 4855 / 1157);
  const word = r(f, a + 6, 14);
  const free = r(f, v(23.9) - 3, 12);
  const cmd = v(25.5);
  const typed = Math.max(0, Math.min(CMD.length, Math.floor((f - cmd) * 0.9)));
  const box = r(f, cmd - 8, 12);
  const fade = r(f, DURATION - 20, 20, E.inOut);
  const enter = r(f, a - 8, 12);
  return (
    <AbsoluteFill style={{ opacity: enter * (1 - fade) }}>
      <div style={{ position: "absolute", left: 960 - lockW / 2, top: 230, width: lockW, height: size, transform: `translateY(${mix(80, 0, r(f, cmd - 10, 16, E.inOut)).toFixed(1)}px)` }}>
        <svg width={size} height={size} style={{ position: "absolute", overflow: "visible" }}>
          {Array.from({ length: 16 }, (_, i) => {
            const col = Math.floor(i / 4);
            const row = i % 4;
            const k = LIT.findIndex(([c, w]) => c === col && w === row);
            const on = k >= 0 ? r(f, a - 4 + k * 1.2, 6, E.pop) : 0;
            return (
              <g key={i}>
                <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr} fill={C.grille} />
                {k >= 0 ? <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr * on} fill={C.lamp} /> : null}
              </g>
            );
          })}
        </svg>
        <Img src={staticFile("logo/word.svg")} style={{ position: "absolute", left: size * (1606.1 / 1157), top: 0, width: size * (4855 / 1157), height: size, opacity: word, transform: `translateX(${((1 - word) * -40).toFixed(1)}px)`, filter: word < 0.99 ? `blur(${((1 - word) * 12).toFixed(1)}px)` : undefined }} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 450, textAlign: "center", ...headStyle(64), fontStretch: "112%", fontWeight: 660, letterSpacing: "-0.02em", opacity: free, transform: `translateY(${mix(80, 0, r(f, cmd - 10, 16, E.inOut)).toFixed(1)}px)`, filter: free < 0.99 ? `blur(${((1 - free) * 12).toFixed(1)}px)` : undefined }}>
        Free and <span style={{ color: C.lamp }}>open source</span>
      </div>
      {/* The call to action: the install command types into a terminal pill, the repo under it. */}
      <div style={{ position: "absolute", left: 960 - 420, top: 600, width: 840, height: 100, borderRadius: 50, background: "#101114", boxShadow: `0 0 0 2px ${EDGE}, 0 30px 80px rgba(0,0,0,.5)`, opacity: box, transform: `scale(${mix(0.9, 1, box).toFixed(3)})`, display: "flex", alignItems: "center", padding: "0 44px", gap: 22, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 38, color: C.paper }}>
        <span style={{ color: C.lamp }}>$</span>
        <span>{CMD.slice(0, typed)}</span>
        <span style={{ width: 18, height: 40, background: C.lamp, opacity: Math.floor(f / 8) % 2 ? 1 : 0.2, marginLeft: -14 }} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 740, textAlign: "center", ...headStyle(46), fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.01em", opacity: r(f, cmd + 18, 12) }}>
        github.com/jasoncronje/spotussi
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 990, textAlign: "center", fontFamily: FONT, fontSize: 22, color: DIM, opacity: r(f, cmd + 26, 12) }}>
        Not affiliated with Spotify. Spotify and Wrapped are trademarks of Spotify AB.
      </div>
    </AbsoluteFill>
  );
};

// ── Audio: voice on top, music under it (ducked 8 dB while she speaks), no effects.
const Music = () => {
  const f = useCurrentFrame();
  const talking = interpolate(f, [v(0) - 8, v(0), v(27.3), v(27.3) + 15], [0, 1, 1, 0], clamp);
  const fade = interpolate(f, [0, 2, DURATION - 40, DURATION], [0, 1, 1, 0], clamp);
  return <Audio src={staticFile("audio/bgm-v3.wav")} volume={fade * (1 - 0.6 * talking)} />;
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
      {bgm ? <Music /> : null}
      <Sequence from={v(0)}>
        <Audio src={staticFile("audio/vo.wav")} />
      </Sequence>
    </AbsoluteFill>
  );
};
