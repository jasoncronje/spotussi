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

export const DURATION = 960;
const VO = 0.3;
// The artist line was re-voiced and spliced in at 9.56 s; it runs 1.34 s longer than the old one,
// so every word after it (original time 12.5 s on) moves by that much.
const SPLICE = 1.34;
const v = (s: number) => Math.round((s + (s >= 12.5 ? SPLICE : 0) + VO) * 30); // voice time → frame
const va = (t: number) => Math.round((9.56 + t + VO) * 30); // a word in the spliced artist line
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
  <div style={{ position: "absolute", background: PANEL, borderRadius: 22, boxShadow: `0 0 0 1.5px ${EDGE}`, overflow: "hidden", ...style }}>{children}</div>
);
const Label = ({ children, style }: { children: React.ReactNode; style?: React.CSSProperties }) => (
  <div style={{ fontFamily: FONT, fontSize: 18, fontWeight: 560, color: MUTE, letterSpacing: "0.01em", ...style }}>{children}</div>
);

type Wd = { t: string; at: number; lit?: boolean };
/** A line of copy appears as one piece when its first word is spoken: a short fade and a small
 * rise, nothing per word. `\n` starts a new line, which appears on its own first word. `out` clears it. */
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
const Stage = ({ f, a, b, children, inD = 8, outD = 6 }: { f: number; a: number; b: number; children: React.ReactNode; inD?: number; outD?: number }) => {
  if (f < a || f >= b) return null;
  return <AbsoluteFill style={{ opacity: r(f, a, inD, E.out) * (1 - r(f, b - outD, outD, E.in)) }}>{children}</AbsoluteFill>;
};

// ── Background: graphite, a slow green glow that breathes with the scenes.
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

const Calendar = ({ f, pages, times, durs, from }: { f: number; pages: { day: number; kind: PageKind }[]; times: number[]; durs: number[]; from?: number }) => {
  let k = 0;
  while (k < times.length && f >= times[k] + durs[k]) k++; // pages fully gone
  const cur = Math.min(k, pages.length - 1);
  const flipping = k < times.length && f >= times[k] ? (f - times[k]) / durs[k] : 0;
  const pFlip = Easing.in(Easing.quad)(Math.min(1, flipping));
  void from;
  return (
    <div style={{ position: "absolute", left: CAL.x, top: CAL.y, width: CAL.w, height: CAL.h, perspective: 1600 }}>
      {/* The pad's thickness: a few page edges under the top page. */}
      {[3, 2, 1].map((i) => <div key={i} style={{ position: "absolute", left: i * 4, right: -i * 4, top: i * 7, bottom: -i * 7, borderRadius: 26, background: "#121316", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }} />)}
      {cur + 1 < pages.length ? <CalPage {...pages[cur + 1]} /> : null}
      <div style={{ position: "absolute", inset: 0, transformOrigin: "50% 0%", transform: `translateY(${(-pFlip * 520).toFixed(1)}px) rotateX(${(-pFlip * 40).toFixed(2)}deg) rotateZ(${(pFlip * -6).toFixed(2)}deg)`, opacity: pFlip >= 1 ? 0 : 1 - Math.max(0, pFlip - 0.5) / 0.5 }}>
        <CalPage {...pages[cur]} />
      </div>
      {/* Binding rings. */}
      {[0.28, 0.72].map((x) => <div key={x} style={{ position: "absolute", left: CAL.w * x - 14, top: -18, width: 28, height: 44, borderRadius: 14, background: "#2f3138", boxShadow: "inset 0 0 0 3px #3f424b" }} />)}
    </div>
  );
};

const Year = ({ f }: { f: number }) => {
  const b = v(6.96);
  if (f > b) return null;
  const once = v(1.06); // "once a year": land on the Wrapped page
  const lockAt = v(2.16); // spotussi: the year again, every page full
  const fast = v(4.38); // "every single day": flat out
  const server = v(5.58); // "on your own server"
  const exit = r(f, b - 7, 7, E.in);

  // Act 1: blank pages Jan → Nov, accelerating, landing on Dec 1, Wrapped.
  const blankDays = Array.from({ length: 8 }, (_, i) => Math.round((i / 8) * (WRAPPED_DAY - 30)));
  const act1 = [...blankDays.map((day) => ({ day, kind: "blank" as PageKind })), { day: WRAPPED_DAY, kind: "wrapped" as PageKind }];
  const s1 = schedule(16, once - 2, act1.length - 1, 1);
  // Act 2: the Wrapped page tears away; Jan 1 onwards, every page full, ending at the server line.
  const fullDays = Array.from({ length: 16 }, (_, i) => Math.round((i / 15) * 364));
  const act2 = fullDays.map((day) => ({ day, kind: "full" as PageKind }));
  const s2 = schedule(lockAt + 22, server - 8, act2.length - 1, 0.8);
  const inAct2 = f >= lockAt;
  const tear = r(f, lockAt, 14, E.in); // the Wrapped page tears off and falls away
  const dayNow = inAct2 ? act2[Math.min(act2.length - 1, s2.times.filter((t) => f >= t).length)].day : 0;

  // Act 3: one move. The pad shrinks and drops through a slot into a small home server whose
  // front panel is the mark's lamp grille; the lamps light as it lands.
  const shrink = r(f, server - 4, 14, E.inOut);
  const drop = r(f, server + 8, 12, E.in);
  const land = server + 20;
  const lamps = r(f, land, 6);
  const device = r(f, server - 4, 10);
  const SLOT_Y = 540;

  return (
    <AbsoluteFill style={{ opacity: 1 - exit, filter: exit > 0 ? `blur(${(exit * 14).toFixed(1)}px)` : undefined }}>
      {/* The pad. */}
      <div style={{ position: "absolute", inset: 0, clipPath: device > 0 ? `inset(0 0 ${1080 - SLOT_Y}px 0)` : undefined }}>
      <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, transform: `translateY(${(mix(0, -220, shrink) + drop * 600).toFixed(1)}px) scale(${mix(1, 0.5, shrink).toFixed(4)})`, transformOrigin: `${CAL.x + CAL.w / 2}px ${CAL.y + CAL.h / 2}px` }}>
        {!inAct2 ? (
          <Calendar f={f} pages={act1} times={s1.times} durs={s1.durs} />
        ) : (
          <>
            <Calendar f={f} pages={act2} times={s2.times} durs={s2.durs} />
            {tear < 1 ? (
              <div style={{ position: "absolute", left: CAL.x, top: CAL.y, width: CAL.w, height: CAL.h, transformOrigin: "100% 0%", transform: `translate(${(tear * 300).toFixed(1)}px, ${(tear * 700).toFixed(1)}px) rotate(${(tear * 28).toFixed(2)}deg)`, opacity: 1 - tear }}>
                <CalPage day={WRAPPED_DAY} kind="wrapped" />
              </div>
            ) : null}
          </>
        )}
      </div>
      </div>
      {/* The home server: one unit, its front panel is the mark. */}
      {device > 0 ? (
        <div style={{ position: "absolute", left: 120, top: SLOT_Y, width: 1680, height: 300, borderRadius: 28, background: "#17181c", boxShadow: `0 0 0 1.5px ${EDGE}`, opacity: device }}>
          <div style={{ position: "absolute", left: CAL.x + CAL.w / 2 - 120 - 170, top: -2, width: 340, height: 12, borderRadius: 6, background: "#050506" }} />
          <svg width={180} height={180} style={{ position: "absolute", left: 70, top: 60 }}>
            {Array.from({ length: 16 }, (_, i) => {
              const col = Math.floor(i / 4);
              const row = i % 4;
              const pitch = 180 / 3.786;
              const rr = pitch * 0.393;
              const lit = [[0, 2], [0, 3], [1, 0], [1, 1], [1, 2], [1, 3], [2, 1], [2, 2], [2, 3], [3, 3]].some(([c, w]) => c === col && w === row);
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
      {/* Copy, left column, in strict turns. */}
      <Say
        f={f}
        size={88}
        out={lockAt - 9}
        style={{ left: 120, top: 400, width: 1040 }}
        words={[
          { t: "Spotify", at: 0 },
          { t: "Wrapped", at: 0 },
          { t: "\n", at: 0 },
          { t: "comes", at: v(0.84) },
          { t: "once", at: v(1.06) },
          { t: "a", at: v(1.46) },
          { t: "year.", at: v(1.66) },
        ]}
      />
      {f >= lockAt ? (
        <div style={{ position: "absolute", left: 120, top: 330, width: 1040, opacity: r(f, lockAt, 10) * (1 - r(f, server - 10, 8, E.in)) }}>
          <Img src={staticFile("logo/lockup-dark.svg")} style={{ height: 64 }} />
          <div style={{ ...headStyle(80), marginTop: 40 }}>
            Your whole history,
            <br />
            <span style={{ color: C.lamp, opacity: r(f, fast, 8) }}>every single day.</span>
          </div>
          <div style={{ fontFamily: FONT, fontSize: 34, fontWeight: 600, color: MUTE, marginTop: 36, fontVariantNumeric: "tabular-nums", opacity: r(f, lockAt + 24, 8) }}>
            Day {dayNow + 1} of 365
          </div>
        </div>
      ) : null}
      <Say f={f} size={96} style={{ left: 120, top: 370 }} words={[{ t: "On", at: v(5.58) }, { t: "your", at: v(5.92) }, { t: "own", at: v(6.1) }, { t: "server.", at: v(6.38) }]} />
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
      <Panel style={{ left: 880, top: 120, width: 920, height: 840 }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(40) }}>Top artists</div>
        {/* The range switch: a glass pill that slides from This month to All time. */}
        <div style={{ position: "absolute", right: 40, top: 30, width: 330, height: 52, borderRadius: 26, background: "#0d0e11", boxShadow: `inset 0 0 0 1.5px ${EDGE}` }}>
          <div style={{ position: "absolute", top: 5, left: mix(5, 167, t), width: 158, height: 42, borderRadius: 21, background: "#2a2c33" }} />
          {["This month", "All time"].map((l, i) => (
            <div key={l} style={{ position: "absolute", top: 15, left: i ? 167 : 5, width: 158, textAlign: "center", fontFamily: FONT, fontSize: 19, fontWeight: 640, color: (i ? t : 1 - t) > 0.5 ? C.paper : MUTE }}>{l}</div>
          ))}
        </div>
        {ARTISTS.map((x, i) => {
          const pos = mix(mRank(x.name), aRank(x.name), t);
          const enter = 1;
          const val = mix(x.month, x.all, t);
          const frac = mix(x.month / maxM, x.all / maxA, t);
          const isOne = aRank(x.name) === 0 && t > 0.6;
          return (
            <div key={x.name} style={{ position: "absolute", left: 40, right: 40, top: 112 + pos * ROW, height: ROW - 8, display: "flex", alignItems: "center", gap: 20, opacity: enter, transform: `translateX(${((1 - enter) * 80).toFixed(1)}px)` }}>
              <div style={{ width: 36, textAlign: "right", ...headStyle(26), color: isOne ? C.lamp : MUTE }}>{Math.round(pos) + 1}</div>
              <Art seed={x.seed} size={46} round />
              <div style={{ width: 290, fontFamily: FONT, fontSize: 24, fontWeight: 620, color: C.paper, whiteSpace: "nowrap" }}>{x.name}</div>
              <div style={{ flex: 1, height: 8, borderRadius: 4, background: "#23252b" }}>
                <div style={{ width: `${(frac * 100).toFixed(2)}%`, height: 8, borderRadius: 4, background: C.lamp }} />
              </div>
              <div style={{ width: 110, textAlign: "right", fontFamily: FONT, fontSize: 24, fontWeight: 680, color: C.paper, fontVariantNumeric: "tabular-nums" }}>{fmt(val)}</div>
            </div>
          );
        })}
        <AbsoluteFill style={{ background: `linear-gradient(0deg, ${PANEL} 0%, rgba(21,22,26,0) 12%)` }} />
      </Panel>
      <Say f={f} size={92} out={sw - 9} style={{ left: 120, top: 420, width: 740 }} words={[{ t: "Your", at: v(6.96) }, { t: "real", at: v(7.28), lit: true }, { t: "\n", at: 0 }, { t: "top", at: v(7.64) }, { t: "10.", at: v(7.9) }]} />
      <Say f={f} size={92} style={{ left: 120, top: 420, width: 740 }} words={[{ t: "Not", at: v(8.3) }, { t: "just", at: v(8.68) }, { t: "\n", at: 0 }, { t: "this", at: v(8.88) }, { t: "month's.", at: v(9.08) }]} />
    </Stage>
  );
};

// ── 4. Your #1 artist: who it is, how many plays all time, and since when.
const ArtistShot = ({ f }: { f: number }) => {
  const a = v(9.56);
  const b = v(12.52);
  const count = r(f, va(1.5) - 2, 30, E.out);
  const first = r(f, va(2.88) - 2, 10);
  const line = 1;
  const rnd = mulberry32(31);
  const yearly = YEARS.map((_, i) => 0.25 + 0.75 * Math.min(1, (0.3 + i / 12) * (0.7 + rnd() * 0.5)));
  return (
    <Stage f={f} a={a} b={b}>
      <Say f={f} size={110} style={{ left: 120, top: 120, width: 1680 }} words={[{ t: "Your", at: va(0) }, { t: "#1", at: va(0.16), lit: true }, { t: "artist.", at: va(0.82) }]} />
      {/* Who: the artist, large, on the left. */}
      <div style={{ position: "absolute", left: 120, top: 360, }}>
        <div>
          <Art seed={1} size={330} round />
        </div>
        <div style={{ ...headStyle(76), marginTop: 40 }}>Nova Reyes</div>
        <div style={{ fontFamily: FONT, fontSize: 30, color: MUTE, marginTop: 10 }}>Synthpop · your most played, all time</div>
      </div>
      {/* The proof: plays all time, the first play, and every year since. */}
      <Panel style={{ left: 880, top: 360, width: 920, height: 600 }}>
        <div style={{ position: "absolute", left: 44, top: 40 }}>
          <Label style={{ fontSize: 24 }}>Plays, all time</Label>
          <div style={{ ...headStyle(150), marginTop: 6, fontVariantNumeric: "tabular-nums", letterSpacing: "-0.05em" }}>{fmt(12418 * count)}</div>
        </div>
        <div style={{ position: "absolute", left: 44, top: 248, display: "flex", alignItems: "baseline", gap: 18, opacity: first, }}>
          <Label style={{ fontSize: 24 }}>First play</Label>
          <div style={{ ...headStyle(44) }}>Mar 4, 2011</div>
        </div>
        <Label style={{ position: "absolute", left: 44, top: 336, fontSize: 22 }}>Plays per year</Label>
        <div style={{ position: "absolute", left: 44, top: 540, width: 832, height: 2, background: EDGE, transform: `scaleX(${line.toFixed(4)})`, transformOrigin: "0 50%" }} />
        {YEARS.map((y, i) => {
          const x = 44 + (i / (YEARS.length - 1)) * 800;
          const on = 1;
          const h = 150 * yearly[i] * on;
          return (
            <div key={y} style={{ position: "absolute", left: x, top: 540 - h, width: 34, height: h, borderRadius: "6px 6px 0 0", background: C.lamp, opacity: 0.5 + 0.5 * (i / (YEARS.length - 1)) }}>
              {i % 5 === 0 || i === YEARS.length - 1 ? <div style={{ position: "absolute", top: h + 12, left: -14, width: 62, textAlign: "center", fontFamily: FONT, fontSize: 20, color: MUTE, opacity: on }}>{y}</div> : null}
            </div>
          );
        })}
      </Panel>
    </Stage>
  );
};

// ── 5. Sessions: lanes of days; each session block builds play by play while a playhead runs.
const Sessions = ({ f }: { f: number }) => {
  const a = v(12.52);
  const b = v(14.94);
  const rnd = mulberry32(55);
  const lanes = ["Mon", "Tue", "Wed", "Thu", "Fri"];
  const head = r(f, a + 6, b - a - 14, (t) => t);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 120, top: 360, width: 1680, height: 600 }}>
        <div style={{ position: "absolute", left: 40, top: 30, ...headStyle(36) }}>Sessions</div>
        {lanes.map((d, li) => {
          const blocks = Array.from({ length: 3 }, (_, k) => {
            const start = 0.02 + k * 0.33 + rnd() * 0.03;
            const len = 0.2 + rnd() * 0.1;
            return { start, len, plays: 8 + Math.floor(rnd() * 6), seed: li * 3 + k };
          });
          return (
            <div key={d} style={{ position: "absolute", left: 40, right: 40, top: 110 + li * 92, height: 72 }}>
              <div style={{ position: "absolute", left: 0, top: 22, fontFamily: FONT, fontSize: 22, color: MUTE }}>{d}</div>
              {blocks.map((bk, k) => {
                const W = 1440;
                const headX = head * W; // playhead position within the lanes
                const grow = 1;
                return (
                  <div key={k} style={{ position: "absolute", left: 90 + bk.start * W, top: 6, width: bk.len * W * grow, height: 60, borderRadius: 12, background: "#1d1f24", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, overflow: "hidden", display: "flex", alignItems: "center", gap: 6, padding: "0 8px" }}>
                    {Array.from({ length: bk.plays }, (_, p) => {
                      const px = bk.start * W + 14 + p * 46; // this play's x
                      return <Art key={p} seed={(bk.seed * 7 + p) % 40} size={40} style={{ opacity: headX >= px ? 1 : 0.12 }} />;
                    })}
                  </div>
                );
              })}
            </div>
          );
        })}
        <div style={{ position: "absolute", top: 90, bottom: 30, left: 130 + head * 1440, width: 3, background: C.lamp }} />
      </Panel>
      <Say f={f} size={92} style={{ left: 120, top: 110, width: 1680 }} words={[{ t: "Every", at: v(12.52) }, { t: "listening", at: v(12.98) }, { t: "session,", at: v(13.44) }, { t: "\n", at: 0 }, { t: "play", at: v(13.98) }, { t: "by", at: v(14.26) }, { t: "play.", at: v(14.5) }]} />
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
      <Panel style={{ left: 880, top: 150, width: 920, height: 780 }}>
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
          const on = 1;
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
      <div style={{ position: "absolute", left: 120, top: 400, width: 740, display: "flex", flexDirection: "column", gap: 30 }}>
        <Say f={f} size={92} style={{ position: "relative" }} words={[{ t: "Watch", at: v(14.94) }, { t: "your", at: v(15.38) }, { t: "\n", at: 0 }, { t: "taste", at: v(15.54) }, { t: "move.", at: v(15.74) }]} />
        <Say f={f} size={46} style={{ position: "relative", color: MUTE, fontStretch: "100%", fontWeight: 520, letterSpacing: "0" }} words={[{ t: "Week", at: v(16.02) }, { t: "on", at: v(16.58) }, { t: "week.", at: v(16.8) }]} />
      </div>
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
  
  const fill = r(f, imp + 6, 26, E.inOut);
  return (
    <Stage f={f} a={a} b={b}>
      <Panel style={{ left: 120, top: 330, width: 820, height: 640 }}>
        <div style={{ position: "absolute", left: 36, top: 30, display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 14, height: 14, borderRadius: 7, background: C.lamp, opacity: 1 }} />
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
      <Panel style={{ left: 980, top: 330, width: 820, height: 640 }}>
        <div style={{ position: "absolute", left: 40, top: 34, ...headStyle(32) }}>Import your history</div>
        <div style={{ position: "absolute", left: 40, top: 110, right: 40, height: 140, borderRadius: 18, boxShadow: `inset 0 0 0 2px ${EDGE}`, display: "flex", alignItems: "center", gap: 22, padding: "0 28px" }}>
          <div style={{ width: 74, height: 90, borderRadius: 10, background: "#22242a", boxShadow: `inset 0 0 0 1.5px ${EDGE}`, display: "flex", alignItems: "flex-end", justifyContent: "center", paddingBottom: 10, fontFamily: FONT, fontSize: 16, fontWeight: 700, color: MUTE, }}>ZIP</div>
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
        {YEARS.map((y, i) => {
          const hh = 40 + 110 * (0.35 + 0.65 * ((i * 37) % 11) / 10);
          const on = Math.min(1, Math.max(0, fill * YEARS.length - i));
          return (
            <div key={y} style={{ position: "absolute", left: 40 + i * 46, bottom: 60, width: 32, height: hh * on, borderRadius: "6px 6px 0 0", background: C.lamp, opacity: 0.45 + 0.55 * on }}>
              {i % 5 === 0 || i === YEARS.length - 1 ? <div style={{ position: "absolute", top: hh * on + 10, left: -14, width: 60, textAlign: "center", fontFamily: FONT, fontSize: 18, color: MUTE }}>{y}</div> : null}
            </div>
          );
        })}
      </Panel>
      <Say f={f} size={80} style={{ left: 120, top: 110, width: 1680 }} words={[{ t: "Every", at: v(17.76) }, { t: "play", at: v(18.12) }, { t: "as", at: v(18.48) }, { t: "it", at: v(18.72) }, { t: "happens.", at: v(18.88) }, { t: "\n", at: 0 }, { t: "And", at: v(19.2) }, { t: "everything", at: v(19.72) }, { t: "before.", at: v(20.2) }]} />
    </Stage>
  );
};

// ── 8. Desktop and pocket: a mini dashboard in a window, the phone slides out in front.
const Devices = ({ f }: { f: number }) => {
  const a = v(20.82);
  const b = v(23.08);
  const ph = r(f, v(22.2) - 6, 18, E.pop);
  
  return (
    <Stage f={f} a={a} b={b}>
      <AbsoluteFill style={{ perspective: 1800 }}>
        <div style={{ position: "absolute", left: 120, top: 290, width: 1240, height: 740, borderRadius: 18, background: "#101114", boxShadow: `0 0 0 1.5px ${EDGE}`, transformOrigin: "0% 50%", overflow: "hidden" }}>
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
              const on = 1;
              return <div key={i} style={{ position: "absolute", left: 24 + i * 26, bottom: 24, width: 16, height: hgt * on, borderRadius: 4, background: C.lamp, opacity: i === 18 ? 1 : 0.62 }} />;
            })}
          </div>
          <div style={{ position: "absolute", left: 810, top: 240, width: 370, height: 480, borderRadius: 14, background: PANEL, boxShadow: `inset 0 0 0 1.5px ${EDGE}`, padding: 20 }}>
            <Label>Top artists</Label>
            {ARTISTS.slice(0, 5).map((x, i) => (
              <div key={x.name} style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 20, opacity: 1 }}>
                <Art seed={x.seed} size={44} round />
                <span style={{ fontFamily: FONT, fontSize: 20, fontWeight: 620, color: C.paper }}>{x.name}</span>
              </div>
            ))}
          </div>
        </div>
      </AbsoluteFill>
      <Phone screen="overview" w={330} style={{ left: mix(1200, 1470, ph), top: mix(420, 260, ph), opacity: Math.min(1, ph * 2), transform: `rotate(${mix(-6, -2, ph).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 60px 120px rgba(0,0,0,.7)` }} />
      <Say f={f} size={80} style={{ left: 120, top: 100, width: 1680 }} words={[{ t: "On", at: v(20.82) }, { t: "your", at: v(21.22) }, { t: "desktop.", at: v(21.36) }, { t: "In", at: v(22.2) }, { t: "your", at: v(22.46) }, { t: "pocket.", at: v(22.6), lit: true }]} />
    </Stage>
  );
};

// ── 9. Sign-off and call to action: the lockup lights lamp by lamp, then the command types out.
const LIT: [number, number][] = [[0, 2], [0, 3], [1, 0], [1, 1], [1, 2], [1, 3], [2, 1], [2, 2], [2, 3], [3, 3]];
const CMD = "docker compose up -d";
const SignOff = ({ f }: { f: number }) => {
  const a = v(23.08);
  if (f < a) return null;
  const size = 150;
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  const lockW = size * (1606.1 / 1157 + 4855 / 1157);
  const word = r(f, a + 2, 10);
  const free = r(f, v(23.9) - 3, 12);
  const cmd = v(25.5);
  const typed = Math.max(0, Math.min(CMD.length, Math.floor((f - cmd) * 0.9)));
  const box = r(f, cmd - 8, 12);
  const fade = r(f, DURATION - 20, 20, E.inOut);
  const enter = r(f, a, 10);
  return (
    <AbsoluteFill style={{ opacity: enter * (1 - fade) }}>
      <div style={{ position: "absolute", left: 960 - lockW / 2, top: 230, width: lockW, height: size, }}>
        <svg width={size} height={size} style={{ position: "absolute", overflow: "visible" }}>
          {Array.from({ length: 16 }, (_, i) => {
            const col = Math.floor(i / 4);
            const row = i % 4;
            const k = LIT.findIndex(([c, w]) => c === col && w === row);
            const on = k >= 0 ? r(f, a + 2, 8) : 0;
            return (
              <g key={i}>
                <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr} fill={C.grille} />
                {k >= 0 ? <circle cx={rr + col * pitch} cy={rr + row * pitch} r={rr * on} fill={C.lamp} /> : null}
              </g>
            );
          })}
        </svg>
        <Img src={staticFile("logo/word.svg")} style={{ position: "absolute", left: size * (1606.1 / 1157), top: 0, width: size * (4855 / 1157), height: size, opacity: word, }} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 450, textAlign: "center", ...headStyle(64), fontStretch: "112%", fontWeight: 660, letterSpacing: "-0.02em", opacity: free }}>
        Free and <span style={{ color: C.lamp }}>open source</span>
      </div>
      {/* The call to action: the install command types into a terminal pill, the repo under it. */}
      <div style={{ position: "absolute", left: 960 - 420, top: 600, width: 840, height: 100, borderRadius: 50, background: "#101114", boxShadow: `0 0 0 2px ${EDGE}`, opacity: box, display: "flex", alignItems: "center", padding: "0 44px", gap: 22, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 38, color: C.paper }}>
        <span style={{ color: C.lamp }}>$</span>
        <span>{CMD.slice(0, typed)}</span>
        <span style={{ width: 18, height: 40, background: C.lamp, opacity: typed < CMD.length ? 1 : 0, marginLeft: -14 }} />
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
  const talking = interpolate(f, [v(0) - 8, v(0), v(27.3), v(27.3) + 15], [0, 1, 1, 0], clamp); // v() includes the splice
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
      <AbsoluteFill style={{ background: "#000", opacity: 1 - r(f, 0, 20, E.inOut) }} />
      {bgm ? <Music /> : null}
      <Sequence from={v(0)}>
        <Audio src={staticFile("audio/vo.wav")} />
      </Sequence>
    </AbsoluteFill>
  );
};
