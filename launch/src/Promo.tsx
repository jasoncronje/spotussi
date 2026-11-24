// Draft 3: the spotussi launch promo. Dark, fast, voice-led.
// Style taken from the owner's references (Lovio, Numtera, LangEase): no pans over flat
// screenshots; single UI cards float in depth, assemble, zoom and hand off to the next scene;
// type carries the message and swaps in place; motion never stops; few, clean sound effects.
//
// 26 s at 30 fps. Voice: ElevenLabs "Laurel" (vo.wav), placed at 1.0 s so its lines land on the
// music (bgm-v3.wav, 124 BPM): "Know your real top ten" on the lift (beat 16), "Live capture" on
// the peak (beat 30, 14.55 s), "spotussi" on beat 42.
import type React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";
import cards from "./cards.json";
import { Figures } from "./Figures";
import { C, FONT } from "./kit";
import { LayoutContext, layouts, type Stop } from "./page";
import { Phone, headStyle } from "./parts";

export const DURATION = 780;
const sec = (s: number) => Math.round(s * 30);
const VO = 1.0; // voice-over start, seconds
const v = (s: number) => sec(s + VO); // a word's start in the voice track → video frame

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
const E = {
  out: Easing.bezier(0.16, 1, 0.3, 1),
  inOut: Easing.bezier(0.65, 0, 0.35, 1),
  in: Easing.bezier(0.7, 0, 0.84, 0),
  fly: Easing.bezier(0.34, 1.3, 0.5, 1),
};
const r = (f: number, a: number, d: number, e: (t: number) => number = E.out) => e(Math.min(1, Math.max(0, (f - a) / Math.max(1, d))));
const mix = (a: number, b: number, t: number) => a + (b - a) * t;
/** 0 before `a`, 1 between, 0 after `b`, with eased edges. Scenes overlap through these. */
const life = (f: number, a: number, b: number, inD = 10, outD = 8) => r(f, a, inD) * (1 - r(f, b - outD, outD, E.in));

// ── Atmosphere: graphite with two soft, slowly drifting glows; green is the only colour.
const Atmos = ({ f }: { f: number }) => {
  const a = Math.sin(f / 70) * 120;
  const b = Math.cos(f / 90) * 140;
  const peak = 1 + 0.5 * life(f, sec(14.4), sec(18), 6, 30);
  return (
    <AbsoluteFill style={{ background: "#08090b" }}>
      <AbsoluteFill style={{ background: `radial-gradient(ellipse 55% 60% at ${50 + a / 40}% ${110 + b / 50}%, rgba(29,185,84,${(0.2 * peak).toFixed(3)}) 0%, rgba(29,185,84,0) 70%)` }} />
      <AbsoluteFill style={{ background: `radial-gradient(ellipse 50% 45% at ${18 - b / 50}% ${-5 + a / 60}%, rgba(90,110,140,0.14) 0%, rgba(90,110,140,0) 70%)` }} />
      <AbsoluteFill style={{ background: "radial-gradient(ellipse 90% 80% at 50% 50%, rgba(0,0,0,0) 55%, rgba(0,0,0,.55) 100%)" }} />
    </AbsoluteFill>
  );
};

// ── A UI card: one panel cut from the app's lit capture, drawn at a display width.
type CardId = keyof typeof cards;
const Card = ({ id, w, style, reveal, figures = false }: { id: CardId; w: number; style?: React.CSSProperties; reveal?: string; figures?: boolean }) => {
  const c = cards[id];
  const k = w / c.w;
  return (
    <div
      style={{
        position: "absolute",
        width: c.w * k,
        height: c.h * k,
        borderRadius: 16,
        overflow: "hidden",
        background: "#111216",
        boxShadow: `0 0 0 1.5px ${C.grille}, 0 40px 100px rgba(0,0,0,.55)`,
        ...style,
      }}
    >
      <div style={{ position: "absolute", left: 0, top: 0, width: c.w, height: c.h, transform: `scale(${k})`, transformOrigin: "0 0", clipPath: reveal }}>
        <Img src={staticFile(`cards/${id}.png`)} style={{ position: "absolute", left: 0, top: 0, width: c.w, height: c.h }} />
        {figures ? (
          <LayoutContext.Provider value={layouts.wide[c.stop as Stop]}>
            <div style={{ position: "absolute", left: -c.x, top: -c.y }}>
              <Figures progress={1} />
            </div>
          </LayoutContext.Provider>
        ) : null}
      </div>
    </div>
  );
};

// ── Type: words blur in at their voice frames and swap in place; the lit word is green.
type W = { t: string; at: number; lit?: boolean };
const Line = ({ words, f, size, style, out, align = "center" }: { words: W[]; f: number; size: number; style?: React.CSSProperties; out?: number; align?: "center" | "left" }) => {
  const gone = out !== undefined ? r(f, out, 7, E.in) : 0;
  if (gone >= 1) return null;
  return (
    <div style={{ position: "absolute", display: "flex", flexWrap: "wrap", justifyContent: align === "center" ? "center" : "flex-start", columnGap: "0.24em", ...headStyle(size), opacity: 1 - gone, filter: gone > 0 ? `blur(${(gone * 14).toFixed(1)}px)` : undefined, transform: `translateY(${(-gone * 30).toFixed(1)}px)`, ...style }}>
      {words.map((w, i) => {
        const p = r(f, w.at - 2, 9);
        const m = w.t.match(/^(.*?)([.,]?)$/) ?? [w.t, w.t, ""];
        return (
          <span key={i} style={{ display: "inline-block", opacity: p, transform: `translateY(${((1 - p) * size * 0.3).toFixed(1)}px) scale(${(1.15 - 0.15 * p).toFixed(3)})`, filter: p > 0.995 ? undefined : `blur(${((1 - p) * 16).toFixed(1)}px)` }}>
            <span style={{ color: w.lit ? C.lamp : undefined }}>{m[1]}</span>
            {m[2]}
          </span>
        );
      })}
    </div>
  );
};

// ── 1. Open: a lamp comes on in the dark, "Every play you've ever made", then "kept."
const Open = ({ f }: { f: number }) => {
  const dot = r(f, 4, 14);
  const lineScale = mix(1.5, 1, r(f, v(0), sec(1.6), E.out));
  const keptAt = v(2.07);
  return (
    <>
      {f < keptAt + 2 ? (
        <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", transform: `scale(${lineScale.toFixed(4)})` }}>
          <Line
            f={f}
            size={110}
            out={keptAt - 6}
            style={{ position: "relative" }}
            words={[
              { t: "Every", at: v(0) },
              { t: "play", at: v(0.16) },
              { t: "you've", at: v(0.52) },
              { t: "ever", at: v(0.84) },
              { t: "made.", at: v(1.16) },
            ]}
          />
        </AbsoluteFill>
      ) : null}
      {/* The opening lamp: alone until the line starts, then it becomes the full stop of "kept." */}
      {f < v(0) + 4 ? (
        <AbsoluteFill style={{ alignItems: "center", justifyContent: "center" }}>
          <span style={{ width: 34, height: 34, borderRadius: "50%", background: C.lamp, opacity: dot * (1 - r(f, v(0) - 2, 6)), transform: `scale(${(0.4 + 0.6 * dot).toFixed(3)})`, boxShadow: `0 0 ${(40 * dot).toFixed(0)}px rgba(29,185,84,.6)` }} />
        </AbsoluteFill>
      ) : null}
    </>
  );
};

// ── 2. "kept." then the lockup assembles: the full stop splits into the mark's ten lamps.
const LOCK = { size: 150 };
const WORD_RATIO = 4855 / 1157;
const WORD_OFFSET = 1606.1 / 1157;
const LIT: [number, number][] = [
  [0, 2], [0, 3], [1, 0], [1, 1], [1, 2], [1, 3], [2, 1], [2, 2], [2, 3], [3, 3],
];
const Kept = ({ f }: { f: number }) => {
  const at = v(2.07);
  const meet = v(2.97);
  const p = r(f, at - 2, 8);
  const away = r(f, meet - 6, 8, E.in);
  if (f < at - 2 || away >= 1) return null;
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", opacity: 1 - away, filter: away > 0 ? `blur(${(away * 12).toFixed(1)}px)` : undefined }}>
      <div style={{ ...headStyle(300), transform: `scale(${(1.25 - 0.25 * p).toFixed(4)})`, opacity: p, filter: p > 0.99 ? undefined : `blur(${((1 - p) * 20).toFixed(1)}px)` }}>
        kept<span style={{ color: C.lamp }}>.</span>
      </div>
    </AbsoluteFill>
  );
};

const Lockup = ({ f, at, cx, cy, size, wordAt }: { f: number; at: number; cx: number; cy: number; size: number; wordAt: number }) => {
  const lockW = size * (WORD_OFFSET + WORD_RATIO);
  const pitch = size / 3.786;
  const rr = pitch * 0.393;
  const word = r(f, wordAt, 12);
  return (
    <div style={{ position: "absolute", left: cx - lockW / 2, top: cy - size / 2, width: lockW, height: size }}>
      <svg width={size} height={size} style={{ position: "absolute", left: 0, top: 0, overflow: "visible" }}>
        {Array.from({ length: 16 }, (_, i) => {
          const col = Math.floor(i / 4);
          const row = i % 4;
          const lit = LIT.some(([c, rw]) => c === col && rw === row);
          const k = LIT.findIndex(([c, rw]) => c === col && rw === row);
          // Lit lamps fly out from one point (the full stop) to their places, a frame apart.
          const t = lit ? r(f, at + k, 12, E.fly) : r(f, at + 6, 10);
          const x = rr + col * pitch;
          const y = rr + row * pitch;
          const ox = lockW / 2 - 0;
          const oy = size / 2;
          return (
            <circle
              key={i}
              cx={lit ? mix(ox, x, t) : x}
              cy={lit ? mix(oy, y, t) : y}
              r={rr}
              fill={lit ? C.lamp : C.grille}
              opacity={lit ? Math.min(1, t * 3) : t}
            />
          );
        })}
      </svg>
      <Img
        src={staticFile("logo/word.svg")}
        style={{ position: "absolute", left: size * WORD_OFFSET, top: 0, width: size * WORD_RATIO, height: size, opacity: word, transform: `translateX(${((1 - word) * -40).toFixed(1)}px)`, filter: word > 0.99 ? undefined : `blur(${((1 - word) * 12).toFixed(1)}px)` }}
      />
    </div>
  );
};

// ── 3. "Meet spotussi. Your Spotify stats, on your own server." The lockup forms, lifts, and
// the dashboard assembles beneath it from its own cards (bento assemble, cards fly in from depth).
const BENTO: { id: CardId; x: number; y: number; w: number; from: [number, number, number]; at: number; figures?: boolean }[] = [
  { id: "ov-tiles", x: 260, y: 300, w: 1400, from: [0, -300, -600], at: 0, figures: true },
  { id: "ov-chart", x: 260, y: 560, w: 980, from: [-600, 200, -900], at: 4 },
  { id: "ov-clock", x: 1268, y: 560, w: 392, from: [700, 100, -900], at: 8 },
  { id: "ov-artists", x: 260, y: 1012, w: 674, from: [-500, 500, -700], at: 12 },
  { id: "ov-tracks", x: 974, y: 1012, w: 674, from: [500, 600, -700], at: 15 },
];
const Meet = ({ f }: { f: number }) => {
  const meet = v(2.97);
  const lift = v(3.93); // "your Spotify stats"
  const end = v(6.79); // "Know"
  const vis = life(f, meet - 4, end + 10, 4, 12);
  if (vis <= 0) return null;
  const up = r(f, lift - 4, 16, E.inOut);
  const lockSize = mix(LOCK.size, 64, up);
  const lockY = mix(540, 120, up);
  const lockX = mix(960, 960, up);
  // The dashboard plane: tilted back, drifting closer the whole time.
  const plane = r(f, lift, end - lift, (t) => t);
  const zoomToArtists = r(f, end - 6, 16, E.in); // hands off to Top: dive into the Top artists card
  return (
    <AbsoluteFill style={{ opacity: vis }}>
      <div style={{ position: "absolute", left: 0, top: 0, opacity: 1 - zoomToArtists }}>
        <Lockup f={f} at={meet + 2} cx={lockX} cy={lockY} size={lockSize} wordAt={v(3.15)} />
      </div>
      <AbsoluteFill style={{ perspective: 1600, perspectiveOrigin: "50% 30%" }}>
        <div
          style={{
            position: "absolute",
            left: 0,
            top: 0,
            width: 1920,
            height: 1080,
            transformStyle: "preserve-3d",
            transform: `translate3d(${mix(0, 330, zoomToArtists)}px, ${mix(60 - plane * 40, -820, zoomToArtists)}px, ${mix(-200 + plane * 120, 700, zoomToArtists)}px) rotateX(${mix(22, 0, zoomToArtists)}deg)`,
            transformOrigin: "50% 0%",
          }}
        >
          {BENTO.map((b) => {
            const t = r(f, lift + b.at, 18, E.out);
            if (f < lift + b.at) return null;
            return (
              <Card
                key={b.id}
                id={b.id}
                w={b.w}
                figures={b.figures}
                style={{
                  left: b.x,
                  top: b.y,
                  opacity: Math.min(1, t * 2.5),
                  transform: `translate3d(${((1 - t) * b.from[0]).toFixed(1)}px, ${((1 - t) * b.from[1]).toFixed(1)}px, ${((1 - t) * b.from[2]).toFixed(1)}px)`,
                  filter: t < 0.97 ? `blur(${((1 - t) * 14).toFixed(1)}px)` : undefined,
                }}
              />
            );
          })}
        </div>
      </AbsoluteFill>
      <AbsoluteFill style={{ background: "linear-gradient(0deg, rgba(8,9,11,.96) 0%, rgba(8,9,11,.85) 16%, rgba(8,9,11,0) 34%)", opacity: r(f, lift, 10) * (1 - zoomToArtists) }} />
      <Line
        f={f}
        size={56}
        out={end - 8}
        style={{ left: 0, right: 0, bottom: 70, fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.02em" }}
        words={[
          { t: "Your", at: v(3.93) },
          { t: "Spotify", at: v(4.37) },
          { t: "stats,", at: v(4.77) },
          { t: "on", at: v(5.11) },
          { t: "your", at: v(5.65), lit: true },
          { t: "own", at: v(5.83), lit: true },
          { t: "server.", at: v(6.13), lit: true },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 4. Top: arrive inside the Top artists card; it opens into the full ranking, bars sweep.
const Top = ({ f }: { f: number }) => {
  const a = v(6.79);
  const b = v(8.37);
  const vis = life(f, a - 4, b + 6, 6, 8);
  if (vis <= 0) return null;
  const open = r(f, a - 4, 16, E.out);
  const drift = r(f, a, b - a + 6, (t) => t);
  const sweep = r(f, a + 4, 22, E.out);
  const exit = r(f, b - 2, 8, E.in);
  return (
    <AbsoluteFill style={{ opacity: vis, perspective: 1400 }}>
      <Card
        id="top-list"
        w={1300}
        reveal={`inset(0 ${((1 - sweep) * 60).toFixed(1)}% 0 0)`}
        style={{
          left: 780,
          top: 150,
          transform: `translateY(${mix(120, -60, drift).toFixed(1)}px) rotateY(${mix(-6, -22, open).toFixed(2)}deg) rotateX(${mix(8, 4, drift).toFixed(2)}deg) scale(${mix(1.25, 1, open).toFixed(4)}) translateX(${(-exit * 400).toFixed(1)}px)`,
          transformOrigin: "100% 50%",
          filter: exit > 0 ? `blur(${(exit * 12).toFixed(1)}px)` : undefined,
        }}
      />
      <Line
        f={f}
        size={112}
        out={b}
        align="left"
        style={{ left: 110, top: 330, width: 640 }}
        words={[
          { t: "Know", at: v(6.79) },
          { t: "your", at: v(7.13) },
          { t: "real", at: v(7.33), lit: true },
          { t: "top", at: v(7.63) },
          { t: "10.", at: v(7.89) },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 5. Artist: the artist page arrives in layers, header from above, chart and clock from below.
const Artist = ({ f }: { f: number }) => {
  const a = v(8.37);
  const b = v(10.35);
  const vis = life(f, a - 4, b + 6, 4, 8);
  if (vis <= 0) return null;
  const t = (d: number) => r(f, a + d, 16, E.out);
  const rise = r(f, a + 10, 30, E.out);
  const orbit = r(f, a, b - a + 6, (x) => x);
  const exit = r(f, b - 2, 8, E.in);
  return (
    <AbsoluteFill style={{ opacity: vis, perspective: 1500, perspectiveOrigin: "50% 40%" }}>
      <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, transformStyle: "preserve-3d", transform: `rotateY(${mix(16, 6, orbit).toFixed(2)}deg) rotateX(${mix(10, 4, orbit).toFixed(2)}deg) translateZ(${(exit * -600).toFixed(0)}px) translateY(${(exit * -500).toFixed(0)}px)` }}>
        <Card id="ar-head" w={1300} figures style={{ left: 380, top: 150, opacity: t(0), transform: `translate3d(0, ${((1 - t(0)) * -260).toFixed(1)}px, 0)` }} />
        <Card id="ar-chart" w={880} reveal={`inset(${((1 - rise) * 100).toFixed(1)}% 0 0 0)`} style={{ left: 380, top: 570, opacity: t(4), transform: `translate3d(0, ${((1 - t(4)) * 300).toFixed(1)}px, 120px)` }} />
        <Card id="ar-clock" w={400} style={{ left: 1290, top: 570, opacity: t(8), transform: `translate3d(${((1 - t(8)) * 300).toFixed(1)}px, ${((1 - t(8)) * 200).toFixed(1)}px, 200px) rotate(${((1 - t(8)) * -40).toFixed(1)}deg)` }} />
      </div>
      <Line
        f={f}
        size={84}
        out={b}
        style={{ left: 0, right: 0, top: 46 }}
        words={[
          { t: "Go", at: v(8.37) },
          { t: "deep", at: v(8.73) },
          { t: "on", at: v(9.03) },
          { t: "any", at: v(9.27) },
          { t: "artist.", at: v(9.67), lit: true },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 6. Sessions: a tabletop of sessions sliding under the camera, meters lighting left to right.
const Sessions = ({ f }: { f: number }) => {
  const a = v(10.35);
  const b = v(12.03);
  const vis = life(f, a - 4, b + 6, 6, 8);
  if (vis <= 0) return null;
  const slide = r(f, a - 4, b - a + 10, (t) => t);
  const lightUp = r(f, a, 26, E.out);
  return (
    <AbsoluteFill style={{ opacity: vis, perspective: 1200, perspectiveOrigin: "50% 10%" }}>
      <Card
        id="se-rows"
        w={1700}
        reveal={`inset(0 ${((1 - lightUp) * 70).toFixed(1)}% 0 0)`}
        style={{ left: 110, top: 220, transform: `rotateX(48deg) rotateZ(-8deg) translateY(${mix(260, -320, slide).toFixed(1)}px) scale(1.15)`, transformOrigin: "50% 0%" }}
      />
      <Line
        f={f}
        size={110}
        out={b}
        style={{ left: 0, right: 0, top: 70 }}
        words={[
          { t: "Relive", at: v(10.35) },
          { t: "every", at: v(10.97) },
          { t: "session.", at: v(11.35), lit: true },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 7. Stats: two charts fan in and their columns rise.
const Stats = ({ f }: { f: number }) => {
  const a = v(12.03);
  const b = v(13.55);
  const vis = life(f, a - 4, b + 2, 4, 6);
  if (vis <= 0) return null;
  const t = (d: number) => r(f, a + d, 14, E.out);
  const rise = (d: number) => r(f, a + d + 4, 20, E.out);
  return (
    <AbsoluteFill style={{ opacity: vis, perspective: 1400 }}>
      <Card id="st-share" w={1180} reveal={`inset(${((1 - rise(4)) * 100).toFixed(1)}% 0 0 0)`} style={{ left: 640, top: 560, opacity: t(4), transform: `translate3d(${((1 - t(4)) * 500).toFixed(0)}px, 0, -200px) rotateY(-18deg)` }} />
      <Card id="st-chart" w={1180} reveal={`inset(${((1 - rise(0)) * 100).toFixed(1)}% 0 0 0)`} style={{ left: 600, top: 170, opacity: t(0), transform: `translate3d(${((1 - t(0)) * 600).toFixed(0)}px, 0, 0) rotateY(-18deg)` }} />
      <Line
        f={f}
        size={118}
        out={b - 4}
        align="left"
        style={{ left: 110, top: 360, width: 560 }}
        words={[
          { t: "Watch", at: v(12.03) },
          { t: "your", at: v(12.43) },
          { t: "taste", at: v(12.61) },
          { t: "move.", at: v(12.85), lit: true },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 8. The peak: a wall of every card racing past in 3D, with the promises swapping in place.
const WALL: CardId[] = ["ov-chart", "ar-clock", "st-share", "ov-albums", "top-list", "ov-genres", "ar-chart", "ov-tracks", "se-rows", "ov-clock", "st-chart", "ov-artists"];
const Peak = ({ f }: { f: number }) => {
  const a = v(13.55);
  const b = v(16.97);
  const vis = life(f, a - 2, b - 1, 3, 6);
  if (vis <= 0) return null;
  const run = f - a;
  const phrases = [
    { t: "Live capture.", at: v(13.55) },
    { t: "Lifetime history.", at: v(14.63) },
    { t: "Works offline.", at: v(16.01) },
  ];
  const cur = [...phrases].reverse().find((p) => f >= p.at - 2) ?? phrases[0];
  const p = r(f, cur.at - 2, 6);
  return (
    <AbsoluteFill style={{ opacity: vis }}>
      <AbsoluteFill style={{ perspective: 1100, perspectiveOrigin: "50% 50%" }}>
        <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, transformStyle: "preserve-3d", transform: "rotateX(24deg) rotateZ(-14deg) translateZ(-300px)" }}>
          {[0, 1, 2, 3].map((col) => {
            const speed = [14, 20, 16, 22][col] * (col % 2 ? 1 : -1);
            return WALL.filter((_, i) => i % 4 === col).flatMap((id, j) =>
              [0, 1].map((rep) => {
                const y = ((j * 520 + rep * 1560 + run * speed) % 3120 + 3120) % 3120 - 1300;
                return <Card key={`${id}-${rep}`} id={id} w={560} style={{ left: -120 + col * 600, top: y, opacity: 0.85 }} />;
              }),
            );
          })}
        </div>
      </AbsoluteFill>
      <AbsoluteFill style={{ background: "radial-gradient(ellipse 60% 40% at 50% 50%, rgba(8,9,11,.92) 0%, rgba(8,9,11,.55) 60%, rgba(8,9,11,.1) 100%)" }} />
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center" }}>
        <div style={{ ...headStyle(170), whiteSpace: "nowrap", transform: `scale(${(1.2 - 0.2 * p).toFixed(4)})`, opacity: p, filter: p > 0.99 ? undefined : `blur(${((1 - p) * 18).toFixed(1)}px)`, display: "flex", alignItems: "center", gap: 40 }}>
          {cur.t === "Live capture." ? <span style={{ width: 46, height: 46, borderRadius: "50%", background: C.lamp, opacity: 0.55 + 0.45 * Math.abs(Math.sin(f / 5)) }} /> : null}
          {cur.t.replace(".", "")}
          <span style={{ color: C.lamp, marginLeft: -36 }}>.</span>
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

// ── 9. Desktop and pocket: the dashboard in a browser window, the phone slides out in front.
const Devices = ({ f }: { f: number }) => {
  const a = v(16.97);
  const b = v(19.35);
  const vis = life(f, a - 1, b + 4, 5, 8);
  if (vis <= 0) return null;
  const win = r(f, a - 1, 18, E.out);
  const phone = r(f, v(17.83), 18, E.fly);
  const drift = r(f, a, b - a, (t) => t);
  return (
    <AbsoluteFill style={{ opacity: vis, perspective: 1600 }}>
      <div style={{ position: "absolute", left: 150, top: 240, transform: `rotateY(${mix(28, 16, drift).toFixed(2)}deg) translateZ(${mix(-300, 0, win).toFixed(0)}px) scale(${mix(0.9, 1, win).toFixed(4)})`, transformOrigin: "0% 50%", opacity: win }}>
        <div style={{ width: 1100, height: 40, background: "#16171b", borderRadius: "14px 14px 0 0", boxShadow: `0 0 0 1.5px ${C.grille}`, display: "flex", alignItems: "center", gap: 8, paddingLeft: 16 }}>
          {[0, 1, 2].map((i) => <span key={i} style={{ width: 11, height: 11, borderRadius: 6, background: C.grille }} />)}
          <span style={{ marginLeft: 340, padding: "3px 60px", borderRadius: 6, background: "#0b0c0f", color: "#8b8f99", fontFamily: FONT, fontSize: 13 }}>spotussi.local</span>
        </div>
        <Card id="ov-top" w={1100} figures style={{ position: "relative", borderRadius: "0 0 14px 14px" }} />
      </div>
      <Phone screen="overview" w={330} style={{ left: mix(900, 1250, phone), top: mix(420, 250, phone), opacity: Math.min(1, phone * 2), transform: `rotate(${mix(-8, -3, phone).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 60px 120px rgba(0,0,0,.7)` }} />
      <Line
        f={f}
        size={78}
        out={b}
        style={{ left: 0, right: 0, top: 70 }}
        words={[
          { t: "On", at: v(16.97) },
          { t: "your", at: v(17.31) },
          { t: "desktop,", at: v(17.47) },
          { t: "and", at: v(17.83) },
          { t: "in", at: v(18.33) },
          { t: "your", at: v(18.65) },
          { t: "pocket.", at: v(18.81), lit: true },
        ]}
      />
    </AbsoluteFill>
  );
};

// ── 10. Sign-off: the lockup assembles on "spotussi", then "Free and open source."
const SignOff = ({ f }: { f: number }) => {
  const a = v(19.35);
  if (f < a - 2) return null;
  const fade = r(f, DURATION - 30, 30, E.inOut);
  const tag = r(f, v(20.25) - 2, 10);
  const foot = r(f, v(22.2), 14);
  return (
    <AbsoluteFill style={{ opacity: 1 - fade }}>
      <Lockup f={f} at={a} cx={960} cy={470} size={190} wordAt={a + 6} />
      <div style={{ position: "absolute", left: 0, right: 0, top: 640, textAlign: "center", ...headStyle(64), fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.02em", opacity: tag, filter: tag > 0.99 ? undefined : `blur(${((1 - tag) * 12).toFixed(1)}px)` }}>
        Free and <span style={{ color: C.lamp }}>open source</span>.
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 760, textAlign: "center", fontFamily: FONT, fontSize: 30, color: "#8b8f99", opacity: foot }}>
        Self-hosted Spotify stats · Desktop and iPhone
      </div>
    </AbsoluteFill>
  );
};

// ── Sound: voice first, music under it (ducked while she speaks), and only a few effects:
// soft swooshes on the big moves, a riser into the peak, a sub boom on the peak and the sign-off.
type Sfx = { at: number; src: string; vol: number };
export const SFX: Sfx[] = [
  { at: v(3.93) - 2, src: "swoosh", vol: 0.5 }, // dashboard assembles
  { at: v(6.79) - 6, src: "swoosh", vol: 0.45 }, // dive into Top
  { at: v(8.37) - 4, src: "swoosh", vol: 0.4 }, // artist page arrives
  { at: v(13.55) - 50, src: "riser", vol: 0.3 }, // build into the peak
  { at: v(13.55), src: "boom", vol: 0.7 }, // the peak
  { at: v(16.97) - 4, src: "swoosh", vol: 0.4 }, // devices
  { at: v(19.35), src: "boom", vol: 0.6 }, // sign-off
];
// Each file's own energy peak, so it lands on its frame.
const PEAK_F: Record<string, number> = { swoosh: 12, riser: 60, boom: 0 };

const Music = ({ bgm }: { bgm: boolean }) => {
  const f = useCurrentFrame();
  if (!bgm) return null;
  const talking = interpolate(f, [sec(VO) - 6, sec(VO), sec(VO + 22.1), sec(VO + 22.1) + 12], [0, 1, 1, 0], clamp);
  const fade = interpolate(f, [0, 3, DURATION - 30, DURATION], [0, 1, 1, 0], clamp);
  return <Audio src={staticFile("audio/bgm-v3.wav")} volume={fade * (1 - 0.55 * talking)} />;
};

export const Launch = ({ bgm = true }: { bgm?: boolean }) => {
  const f = useCurrentFrame();
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Atmos f={f} />
      <Open f={f} />
      <Kept f={f} />
      <Meet f={f} />
      <Top f={f} />
      <Artist f={f} />
      <Sessions f={f} />
      <Stats f={f} />
      <Peak f={f} />
      <Devices f={f} />
      <SignOff f={f} />
      <Music bgm={bgm} />
      <Sequence from={sec(VO)}>
        <Audio src={staticFile("audio/vo.wav")} volume={1} />
      </Sequence>
      {SFX.map((s, i) => (
        <Sequence key={i} from={Math.max(0, s.at - (PEAK_F[s.src] ?? 0))}>
          <Audio src={staticFile(`audio/sfx/${s.src}.mp3`)} volume={s.vol} />
        </Sequence>
      ))}
    </AbsoluteFill>
  );
};
