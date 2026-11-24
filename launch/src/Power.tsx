// "Power on": the spotussi launch film. The desktop app boots like a hi-fi amp:
// meters self-test, the dashboard lights as the camera cranes back, the drop
// throws every page past a steep 3D wall, then a run of pages each light their
// own instrument on the beat. The phone is a short coda; the lockup closes.
//
// 44.3 s at 30 fps, cut to bgm-power.mp3 (128 BPM, first beat 0.434 s, drop at beat 30).
import type React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";
import { Chart, Sprite, ignite } from "./Lamps";
import { Figures } from "./Figures";
import { C, EASE, FONT, ramp } from "./kit";
import { LayoutContext, layouts, type Lamp, type Layout, type Stop } from "./page";
import { Mark, Phone, Strip, Words, headStyle, subStyle, words } from "./parts";

// ── Beat grid: 128 BPM is 14.0625 frames a beat.
const BEAT0 = 13.02;
const PB = 14.0625;
const B = (n: number) => Math.round(BEAT0 + PB * n);

export const SHOTS = {
  selftest: { from: 0, to: B(8) }, // meters self-test, macro
  words: { from: B(8), to: B(14) }, // one word a beat
  crane: { from: B(14), to: B(30) }, // the dashboard lights as the camera rises
  wall: { from: B(30), to: B(38) }, // the drop: every page past a steep wall
  top: { from: B(38), to: B(46) }, // crash zoom onto the #1 artist
  artist: { from: B(46), to: B(54) },
  sessions: { from: B(54), to: B(62) },
  stats: { from: B(62), to: B(70) },
  pocket: { from: B(70), to: B(76) }, // the phone coda
  lockup: { from: B(76), to: 1252 },
} as const;
export const DURATION = SHOTS.lockup.to;
type Span = { from: number; to: number };
const len = (s: Span) => s.to - s.from;
const rel = (s: Span, n: number) => B(n) - s.from; // beat n, relative to a shot

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
const mix = (a: number, b: number, t: number) => a + (b - a) * t;
const W = layouts.wide;

// ── A captured page: the dark capture, with lit layers laid over in page pixels.
const Page = ({ stop, children, lit = 0, crop, litOnly = false }: { stop: Stop; children?: React.ReactNode; lit?: number; crop?: number; litOnly?: boolean }) => {
  const L = W[stop];
  const h = crop ?? L.page.h;
  return (
    <LayoutContext.Provider value={L}>
      <div style={{ position: "absolute", left: 0, top: 0, width: L.page.w, height: h, overflow: "hidden", background: "#0f1013" }}>
        {litOnly ? null : <Img src={L.dark} style={{ position: "absolute", left: 0, top: 0, width: L.page.w, height: L.page.h }} />}
        {/* The full lit pass catches anything the lamp lists don't; it sits under the overlays so the
            counted figures (empty slots in both captures) stay on top. */}
        {lit > 0 || litOnly ? <Img src={L.lit} style={{ position: "absolute", left: 0, top: 0, width: L.page.w, height: L.page.h, opacity: litOnly ? 1 : lit }} /> : null}
        {litOnly ? <Figures progress={1} /> : null}
        {children}
      </div>
    </LayoutContext.Provider>
  );
};

/** Camera over page pixels: (fx, fy) lands at the frame centre, at scale s. */
const Cam = ({ s, fx, fy, children, style }: { s: number; fx: number; fy: number; children: React.ReactNode; style?: React.CSSProperties }) => (
  <div
    style={{
      position: "absolute",
      left: 0,
      top: 0,
      transformOrigin: "0 0",
      transform: `translate(${(960 - fx * s).toFixed(2)}px, ${(540 - fy * s).toFixed(2)}px) scale(${s.toFixed(5)})`,
      ...style,
    }}
  >
    {children}
  </div>
);

/** Lamps lit at explicit frames, each with the incandescent flash. */
const LampsAt = ({ lamps, at }: { lamps: Lamp[]; at: (l: Lamp, i: number) => number }) => {
  const frame = useCurrentFrame();
  return (
    <>
      {lamps.map((l, i) => {
        const t = at(l, i);
        if (frame < t) return null;
        if (l.kind === "bar") {
          const p = interpolate(frame, [t, t + 16], [0, 100], { ...clamp, easing: EASE.snappy });
          return <Sprite key={i} rect={l} style={{ clipPath: `inset(0 ${100 - p}% 0 0)` }} />;
        }
        return <Sprite key={i} rect={l} style={ignite(frame, t)} />;
      })}
    </>
  );
};

const allLamps = (L: Layout): Lamp[] => [...L.pilots, ...L.meters.flat(), ...L.dots, ...L.clockLamps, ...L.rankBars];

const Vignette = ({ k = 0.65 }: { k?: number }) => (
  <AbsoluteFill style={{ background: `radial-gradient(ellipse 75% 70% at 50% 50%, rgba(0,0,0,0) 55%, rgba(0,0,0,${k}) 100%)` }} />
);

const Flash = ({ at, peak = 0.9, dur = 5 }: { at: number; peak?: number; dur?: number }) => {
  const frame = useCurrentFrame();
  const o = interpolate(frame, [at, at + dur], [peak, 0], { ...clamp, easing: EASE.out });
  return frame >= at && o > 0 ? <AbsoluteFill style={{ background: C.paper, opacity: o }} /> : null;
};

// ── Caption styles: each feature shot speaks differently, briefly, then clears the frame for the UI.
const useInOut = (at: number, until: number, inDur = 8, outDur = 8) => {
  const frame = useCurrentFrame();
  return { frame, p: ramp(frame, at, inDur, EASE.snappy) * (1 - ramp(frame, until, outDur, EASE.exit)) };
};

/** Top: a giant lit "#1" slams in beside the crash target, the line under it. */
const RankSlam = ({ at }: { at: number }) => {
  const { frame, p } = useInOut(at, at + 46);
  const hit = ramp(frame, at, 5, EASE.snappy);
  return (
    <>
      <AbsoluteFill style={{ background: "linear-gradient(270deg, rgba(10,11,13,.97) 0%, rgba(10,11,13,.85) 36%, rgba(10,11,13,0) 58%)", opacity: p }} />
      <div style={{ position: "absolute", right: 110, top: 330, textAlign: "right", opacity: p }}>
        <div style={{ ...headStyle(380), color: C.lamp, lineHeight: 0.9, transform: `scale(${(1.4 - 0.4 * hit).toFixed(4)})`, transformOrigin: "100% 50%", filter: hit > 0.999 ? undefined : `blur(${((1 - hit) * 18).toFixed(1)}px)` }}>#1</div>
        <div style={{ ...headStyle(64), marginTop: 18 }}>Know your real top 10.</div>
      </div>
    </>
  );
};

/** Artist: a small console label, top right, like a channel strip's legend. */
const Label = ({ text, at, until }: { text: string; at: number; until: number }) => {
  const { p } = useInOut(at, until);
  return (
    <div style={{ position: "absolute", right: 90, top: 90, padding: "22px 34px", borderRadius: 18, background: "rgba(14,15,18,.9)", boxShadow: `0 0 0 2px ${C.grille}`, opacity: p, transform: `translateY(${((1 - p) * -20).toFixed(2)}px)` }}>
      <Words items={words(text, at, 2, 12)} dy={18} blur={8} style={{ ...headStyle(62), flexWrap: "nowrap" }} />
    </div>
  );
};

/** Sessions: the line types out across the top, a character a frame, with a lamp for a cursor. */
const Typed = ({ text, lit, at, until }: { text: string; lit: string; at: number; until: number }) => {
  const { frame, p } = useInOut(at, until, 2);
  const n = Math.max(0, Math.min(text.length, Math.floor((frame - at) * 1.2)));
  const shown = text.slice(0, n);
  const k = text.indexOf(lit);
  return (
    <>
      <AbsoluteFill style={{ background: "linear-gradient(180deg, rgba(10,11,13,.95) 0%, rgba(10,11,13,.5) 22%, rgba(10,11,13,0) 38%)", opacity: p }} />
      <div style={{ position: "absolute", left: 0, right: 0, top: 96, textAlign: "center", opacity: p, ...headStyle(100) }}>
        {shown.slice(0, k)}
        <span style={{ color: C.lamp }}>{shown.slice(k)}</span>
        <span style={{ display: "inline-block", width: 34, height: 34, borderRadius: "50%", background: C.lamp, marginLeft: 18, opacity: n < text.length ? 1 : 0 }} />
      </div>
    </>
  );
};

/** Stats: one huge word in the middle, a small lead-in above it. */
const BigCenter = ({ small, big, lit, at, until }: { small: string; big: string; lit: string; at: number; until: number }) => {
  const { frame, p } = useInOut(at, until);
  const hit = ramp(frame, at + 3, 6, EASE.snappy);
  return (
    <>
      <AbsoluteFill style={{ background: "radial-gradient(ellipse 60% 45% at 50% 52%, rgba(10,11,13,.9) 0%, rgba(10,11,13,.55) 60%, rgba(10,11,13,0) 100%)", opacity: p }} />
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", opacity: p }}>
        <div style={{ ...headStyle(56), color: C.mist, marginBottom: 6 }}>{small}</div>
        <div style={{ ...headStyle(230), transform: `scale(${(1.25 - 0.25 * hit).toFixed(4)})`, filter: hit > 0.999 ? undefined : `blur(${((1 - hit) * 16).toFixed(1)}px)`, whiteSpace: "nowrap" }}>
          {big} <span style={{ color: C.lamp }}>{lit.replace(".", "")}</span>.
        </div>
      </AbsoluteFill>
    </>
  );
};

// ── The source selector: between pages the amp's own selector clicks over, landing on the beat of the cut.
const SOURCES = ["OVERVIEW", "TOP", "ARTIST", "SESSIONS", "STATS"];
const SEL = 12; // frames the selector is on screen before each cut
const Selector = ({ from, to }: { from: number; to: number }) => {
  const frame = useCurrentFrame();
  const turn = Easing.bezier(0.34, 1.5, 0.5, 1)(Math.min(1, Math.max(0, (frame - (SEL - 6)) / 6)));
  const ang = (i: number) => -64 + i * 32;
  const a = mix(ang(from), ang(to), turn);
  const on = ramp(frame, 0, 3, EASE.out);
  const R0 = 250;
  return (
    <AbsoluteFill style={{ background: "#0a0b0d", opacity: on }}>
      <div style={{ position: "absolute", left: 960, top: 640 }}>
        {/* Legends round the top of the dial; the selected one lights. */}
        {SOURCES.map((t, i) => {
          const r = ((ang(i) - 90) * Math.PI) / 180;
          const lit = i === to && turn > 0.85;
          return (
            <div key={t} style={{ position: "absolute", left: Math.cos(r) * (R0 + 110 + 70 * Math.abs(Math.sin(r * 0 + ((ang(i) * Math.PI) / 180)))), top: Math.sin(r) * (R0 + 110), transform: "translate(-50%,-50%)", ...headStyle(30), fontStretch: "112%", letterSpacing: "0.08em", color: lit ? C.lamp : i === from && turn < 0.5 ? C.paper : "#5B5F68" }}>
              {t}
            </div>
          );
        })}
        {/* Tick lamps between the legends. */}
        {Array.from({ length: 17 }, (_, k) => {
          const deg = -64 + k * 8;
          const r = ((deg - 90) * Math.PI) / 180;
          const lit = Math.abs(deg - a) < 4.5;
          return <span key={k} style={{ position: "absolute", left: Math.cos(r) * (R0 + 44) - 7, top: Math.sin(r) * (R0 + 44) - 7, width: 14, height: 14, borderRadius: "50%", background: lit ? C.lamp : C.grille }} />;
        })}
        {/* The knob: graphite, a fine edge, a lit lamp for its pointer. */}
        <div style={{ position: "absolute", left: -R0, top: -R0, width: R0 * 2, height: R0 * 2, borderRadius: "50%", background: "radial-gradient(circle at 40% 32%, #2a2c33 0%, #1A1C21 55%, #121317 100%)", boxShadow: `0 0 0 3px ${C.grille}, 0 40px 90px rgba(0,0,0,.6)`, transform: `rotate(${a.toFixed(2)}deg)` }}>
          <span style={{ position: "absolute", left: R0 - 16, top: 34, width: 32, height: 32, borderRadius: "50%", background: C.lamp }} />
        </div>
      </div>
    </AbsoluteFill>
  );
};
const SELECTS = [
  { at: B(38), from: 0, to: 1 },
  { at: B(46), from: 1, to: 2 },
  { at: B(54), from: 2, to: 3 },
  { at: B(62), from: 3, to: 4 },
];

// ── 1. Self-test: a macro on each Overview meter. It sweeps on, the figure counts, the camera hops on.
const OV = W.overview;
const meterBox = (i: number) => {
  const m = OV.meters[i];
  const f = OV.figures[i];
  const x0 = Math.min(f.x, ...m.map((l) => l.x));
  const x1 = Math.max(f.x + f.w, ...m.map((l) => l.x + l.w));
  const y0 = f.y;
  const y1 = Math.max(...m.map((l) => l.y + l.h));
  return { cx: (x0 + x1) / 2, cy: (y0 + y1) / 2 };
};
const SWEEP = (i: number) => B(1 + 2 * i); // meter i sweeps on beat 1, 3, 5, 7

const SelfTest = () => {
  const frame = useCurrentFrame();
  // Hop between meters on the off-beats: hold, then a quick smooth pan.
  let fx = meterBox(0).cx;
  let fy = meterBox(0).cy + 40; // keep the page title out of frame
  for (let i = 1; i < 4; i++) {
    const p = ramp(frame, SWEEP(i) - 9, 9, EASE.smooth);
    fx = mix(fx, meterBox(i).cx, p);
    fy = mix(fy, meterBox(i).cy + 40, p);
  }
  const breathe = 3.4 * (1 + 0.03 * ramp(frame, 0, len(SHOTS.selftest), EASE.smooth));
  const on = ramp(frame, 0, 14, EASE.out); // fade up from black
  const exit = ramp(frame, len(SHOTS.selftest) - 5, 5, EASE.exit);
  return (
    <AbsoluteFill style={{ opacity: on * (1 - exit) }}>
      <Cam s={breathe} fx={fx} fy={fy}>
        <Page stop="overview">
          {OV.meters.map((m, i) => (
            <LampsAt key={i} lamps={m} at={(_, k) => SWEEP(i) + Math.round((k / Math.max(1, m.length - 1)) * 7)} />
          ))}
          <Figures progress={0} progressOf={(i) => ramp(frame, SWEEP(i), 18, EASE.out)} />
        </Page>
      </Cam>
      <Vignette k={0.85} />
    </AbsoluteFill>
  );
};

// ── 2. One word a beat, on black.
const WORDS = [
  { t: "Every", beat: 8 },
  { t: "play,", beat: 9 },
  { t: "kept", beat: 10 },
  { t: "forever.", beat: 11, lit: true },
];
const WordBeats = () => {
  const frame = useCurrentFrame();
  const f = frame + SHOTS.words.from;
  const lineAt = B(12);
  if (f >= lineAt) {
    const p = ramp(f, lineAt, 10, EASE.snappy);
    const push = 1 + 0.12 * ramp(f, lineAt, B(14) - lineAt, EASE.smooth);
    return (
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center" }}>
        <div style={{ ...headStyle(112), transform: `scale(${(push * (1.08 - 0.08 * p)).toFixed(4)})`, opacity: p, filter: p > 0.999 ? undefined : `blur(${((1 - p) * 10).toFixed(2)}px)`, whiteSpace: "nowrap" }}>
          Every play, kept <span style={{ color: C.lamp }}>forever</span>.
        </div>
      </AbsoluteFill>
    );
  }
  const cur = [...WORDS].reverse().find((w) => f >= B(w.beat)) ?? WORDS[0];
  const p = ramp(f, B(cur.beat), 6, EASE.snappy);
  const txt = cur.lit ? cur.t.replace(".", "") : cur.t;
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center" }}>
      <div style={{ ...headStyle(330), transform: `scale(${(1.22 - 0.22 * p).toFixed(4)})`, filter: p > 0.999 ? undefined : `blur(${((1 - p) * 16).toFixed(2)}px)`, opacity: Math.min(1, p * 2) }}>
        <span style={{ color: cur.lit ? C.lamp : undefined }}>{txt}</span>
        {cur.lit ? "." : null}
      </div>
    </AbsoluteFill>
  );
};

// ── 3. Crane: start tight on the plays-by-day peak, rise until the whole dashboard is on screen.
// Every lamp lights the moment it comes into view (crane-rise-reveal's row wash, in lamps).
const PEAK_COL = OV.columns.reduce((a, c) => (c.top < a.top ? c : a), OV.columns[0]);
const CR = { hold: rel(SHOTS.crane, 18), end: rel(SHOTS.crane, 27), s0: 2.4, s1: 0.86 };
const F0 = { x: PEAK_COL.x + 140, y: (PEAK_COL.top + PEAK_COL.base) / 2 };
const F1 = { x: 720, y: 640 };
const craneEase = Easing.bezier(0.2, 0, 0.25, 1);
const craneAt = (frame: number) => {
  const e = craneEase(Math.min(1, Math.max(0, (frame - CR.hold) / (CR.end - CR.hold))));
  const settle = ramp(frame, CR.end, len(SHOTS.crane) - CR.end, EASE.smooth);
  const s = mix(CR.s0 * (1 + 0.02 * ramp(frame, 0, CR.hold, EASE.smooth)), CR.s1, e) * (1 + 0.1 * settle); // keep pushing in through the riser
  return { s, fx: mix(F0.x, F1.x, e), fy: mix(F0.y, F1.y, e) };
};
const seen = (l: { x: number; y: number; w: number; h: number }, frame: number) => {
  const { s, fx, fy } = craneAt(frame);
  const cx = l.x + l.w / 2;
  const cy = l.y + l.h / 2;
  return Math.abs(cx - fx) < 960 / s && Math.abs(cy - fy) < 540 / s;
};
const firstSeen = (l: { x: number; y: number; w: number; h: number }) => {
  for (let f = 0; f <= CR.end + 4; f++) if (seen(l, f)) return f;
  return CR.end + 4;
};
const CRANE_LAMPS = allLamps(OV);
const CRANE_AT = CRANE_LAMPS.map((l) => Math.max(6, firstSeen(l) + 2));
const FIG_AT = OV.figures.map((f) => firstSeen(f));

const Crane = () => {
  const frame = useCurrentFrame();
  const { s, fx, fy } = craneAt(frame);
  const fullLit = ramp(frame, CR.end, 14, EASE.out);
  return (
    <AbsoluteFill>
      <Cam s={s} fx={fx} fy={fy}>
        <Page stop="overview" lit={fullLit}>
          <Chart stagger={0.035} climb={0.55} />
          <LampsAt lamps={CRANE_LAMPS} at={(_, i) => CRANE_AT[i]} />
          <Figures progress={0} progressOf={(i) => ramp(frame, FIG_AT[i], 24, EASE.out)} />
        </Page>
      </Cam>
      <Vignette k={0.55} />
    </AbsoluteFill>
  );
};

// ── 4. The drop: five lit pages as one train, sliding past a fixed camera at 60° (steep-tilt-glide).
const TRAIN: Stop[] = ["top", "artist", "sessions", "stats", "history"];
const CAR = 1440 + 140;
const CAR_H = 1150;
const glide = Easing.bezier(0.3, 0.12, 0.72, 0.9);
const trainX = (frame: number) => mix(380, -(TRAIN.length - 1) * CAR - 300, glide(Math.min(1, Math.max(0, frame / len(SHOTS.wall)))));

const TrainStrip = ({ x }: { x: number }) => (
  <div style={{ position: "absolute", left: 0, top: 0, transform: `translateX(${x.toFixed(2)}px)` }}>
    {TRAIN.map((stop, i) => (
      <div key={stop} style={{ position: "absolute", left: i * CAR, top: 0, width: 1440, height: CAR_H, borderRadius: 18, overflow: "hidden", boxShadow: `0 0 0 3px ${C.grille}` }}>
        <Page stop={stop} litOnly crop={CAR_H} />
      </div>
    ))}
  </div>
);

const Wall = () => {
  const frame = useCurrentFrame();
  const x = trainX(frame);
  const v = Math.abs(x - trainX(frame - 1));
  const exit = ramp(frame, len(SHOTS.wall) - 5, 5, EASE.exit);
  const dim = 0.35 + 0.65 * ramp(frame, 0, 30, EASE.out); // reveal from dark
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <AbsoluteFill style={{ perspective: 1100, perspectiveOrigin: "30% 58%" }}>
        <div style={{ position: "absolute", left: "38%", top: "12%", transform: "rotateY(-60deg) rotateZ(-2deg)", transformOrigin: "left top", filter: `brightness(${dim.toFixed(3)})` }}>
          <div style={{ transform: "scale(0.62)", transformOrigin: "left top" }}>
            {/* Speed ghosts: where the train was 1.6, 3.2 and 4.8 frames ago, fading with speed. */}
            {[2.4].map((d) => (
              <div key={d} style={{ position: "absolute", opacity: Math.min(0.35, v / 200) }}>
                <TrainStrip x={trainX(frame - d)} />
              </div>
            ))}
            <TrainStrip x={x} />
          </div>
        </div>
      </AbsoluteFill>
      <AbsoluteFill style={{ background: "linear-gradient(90deg, rgba(10,11,13,.95) 0%, rgba(10,11,13,.6) 30%, rgba(10,11,13,0) 52%)" }} />
      <div style={{ position: "absolute", left: 120, top: 330 }}>
        <Words items={words("Every artist,", 4, 3, 14)} dy={40} blur={14} style={headStyle(112)} />
        <Words items={words("album, *track.", 10, 3, 14)} dy={40} blur={14} style={headStyle(112)} />
        <Words items={words("Your whole history, live and imported.", 18, 2, 16)} dy={16} blur={8} style={{ ...subStyle(36), marginTop: 36 }} />
      </div>
      <Flash at={0} peak={0.6} dur={6} />
    </AbsoluteFill>
  );
};

// ── 5. Top: wide on the list while rank bars sweep in, then a 6-frame crash zoom onto the #1 artist.
const TOP = W.top;
const ROW1 = TOP.firstRow ?? { x: 65, y: 294, w: 1310, h: 60 };
const CRASH = rel(SHOTS.top, 42); // arrival frame, on beat 42
const TopShot = () => {
  const frame = useCurrentFrame();
  const wide = { s: 1.25, fx: 720, fy: 520 };
  const tight = { s: 2.5, fx: ROW1.x + 390, fy: ROW1.y + ROW1.h / 2 + 70 };
  const back = ramp(frame, CRASH - 13, 7, EASE.smooth); // pull back 4% first
  const go = Easing.in(Easing.cubic)(Math.min(1, Math.max(0, (frame - (CRASH - 6)) / 6)));
  const bounce = frame >= CRASH ? 1 + 0.05 * Math.exp(-(frame - CRASH) / 2.2) * Math.cos((frame - CRASH) * 0.9) : 1;
  const s = mix(wide.s * (1 - 0.04 * back), tight.s, go) * bounce;
  const fx = mix(wide.fx, tight.fx, go);
  const fy = mix(wide.fy, tight.fy, go);
  const blur = frame >= CRASH - 6 && frame < CRASH ? 5 : 0;
  const exit = ramp(frame, len(SHOTS.top) - 5, 5, EASE.exit);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <Cam s={s} fx={fx} fy={fy} style={{ filter: blur ? `blur(${blur}px)` : undefined }}>
        <Page stop="top" lit={ramp(frame, 30, 10, EASE.out)}>
          <LampsAt lamps={[...TOP.pilots, ...TOP.rankBars, ...TOP.dots]} at={(l, i) => (l.kind === "bar" ? 4 + i * 2 : 2)} />
        </Page>
      </Cam>
      <Vignette k={0.6} />
      <RankSlam at={CRASH + 2} />
    </AbsoluteFill>
  );
};

// ── 6. Artist: the chart climbs column by column and the listening clock lights around the dial.
const AR = W.artist;
const ArtistShot = () => {
  const frame = useCurrentFrame();
  const push = ramp(frame, 0, len(SHOTS.artist), EASE.smooth);
  const s = 1.15 + 0.18 * push;
  const fx = mix(780, 900, push);
  const fy = mix(470, 560, push);
  const exit = ramp(frame, len(SHOTS.artist) - 5, 5, EASE.exit);
  const clockStart = rel(SHOTS.artist, 48);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <Cam s={s} fx={fx} fy={fy}>
        <Page stop="artist" lit={ramp(frame, rel(SHOTS.artist, 52), 10, EASE.out)}>
          <Chart stagger={0.045} climb={0.5} />
          <LampsAt lamps={[...AR.pilots, ...AR.dots, ...AR.rankBars]} at={(_, i) => 2 + (i % 12)} />
          <LampsAt lamps={AR.clockLamps} at={(_, i) => clockStart + Math.round((i / Math.max(1, AR.clockLamps.length - 1)) * PB * 2)} />
          <Figures progress={0} progressOf={() => ramp(frame, 4, 26, EASE.out)} />
        </Page>
      </Cam>
      <Vignette k={0.6} />
      <Label text="Go deep on any *artist." at={rel(SHOTS.artist, 46) + 4} until={rel(SHOTS.artist, 50)} />
    </AbsoluteFill>
  );
};

// ── 7. Sessions: the page laid back like a tabletop, scrolling, its session meters lighting in a wave.
const SE = W.sessions;
const SessionsShot = () => {
  const frame = useCurrentFrame();
  const scroll = ramp(frame, 0, len(SHOTS.sessions), (t) => t);
  const exit = ramp(frame, len(SHOTS.sessions) - 5, 5, EASE.exit);
  const x0 = Math.min(...SE.dots.map((l) => l.x));
  const y0 = Math.min(...SE.dots.map((l) => l.y));
  return (
    <AbsoluteFill style={{ opacity: 1 - exit, perspective: 1300, perspectiveOrigin: "50% 20%" }}>
      <div style={{ position: "absolute", left: 960 - 720 * 1.15, top: 80, transform: `rotateX(34deg) rotateZ(-6deg) scale(1.15)`, transformOrigin: "50% 0%" }}>
        <div style={{ transform: `translateY(${(-60 - 520 * scroll).toFixed(2)}px)` }}>
          <Page stop="sessions" lit={ramp(frame, len(SHOTS.sessions) - 30, 14, EASE.out)}>
            <LampsAt lamps={[...SE.pilots, ...SE.dots]} at={(l) => 4 + Math.round(((l.x - x0) * 0.45 + (l.y - y0) * 0.9) / 1000 * 30)} />
          </Page>
        </div>
      </div>
      <Vignette k={0.7} />
      <Typed text="Every session, remembered." lit="remembered" at={rel(SHOTS.sessions, 54) + 6} until={rel(SHOTS.sessions, 59)} />
    </AbsoluteFill>
  );
};

// ── 8. Stats: a lateral dolly across the charts as every column climbs.
const StatsShot = () => {
  const frame = useCurrentFrame();
  const dolly = ramp(frame, 0, len(SHOTS.stats), EASE.smooth);
  const exit = ramp(frame, len(SHOTS.stats) - 6, 6, EASE.exit);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <Cam s={1.45} fx={mix(560, 940, dolly)} fy={mix(470, 560, dolly)}>
        <Page stop="stats" lit={ramp(frame, 40, 20, EASE.out)}>
          <Chart stagger={0.03} climb={0.5} />
        </Page>
      </Cam>
      <Vignette k={0.6} />
      <BigCenter small="Watch your" big="taste" lit="move." at={rel(SHOTS.stats, 63)} until={rel(SHOTS.stats, 66)} />
    </AbsoluteFill>
  );
};

// ── 9. Pocket: the desktop steps back into a window and the phone slides out from behind it.
const Pocket = () => {
  const frame = useCurrentFrame();
  const shrink = ramp(frame, 0, 18, EASE.out);
  const out = ramp(frame, 8, 22, EASE.snappy);
  const exit = ramp(frame, len(SHOTS.pocket) - 5, 5, EASE.exit);
  const ws = 0.56;
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <div style={{ position: "absolute", left: 140, top: 180, transform: `scale(${(1.12 - 0.12 * shrink).toFixed(4)})`, transformOrigin: "50% 50%", opacity: shrink }}>
        <div style={{ position: "relative", width: 1440 * ws, height: 1260 * ws, borderRadius: 14, overflow: "hidden", boxShadow: `0 0 0 2px ${C.grille}, 0 50px 120px rgba(0,0,0,.6)` }}>
          <div style={{ transform: `scale(${ws})`, transformOrigin: "0 0" }}>
            <Page stop="overview" lit={1} crop={1260} />
          </div>
        </div>
      </div>
      <Phone screen="overview" w={340} style={{ left: mix(560, 900, out), top: mix(330, 190, out), transform: `rotate(${mix(-4, 3, out).toFixed(2)}deg)`, boxShadow: `0 0 0 2.5px ${C.grille}, 0 50px 120px rgba(0,0,0,.65)` }} />
      <div style={{ position: "absolute", left: 1310, top: 400, width: 600 }}>
        <Words items={words("Spotify stats,", 12, 3, 16)} dy={30} blur={12} style={headStyle(72)} />
        <Words items={words("in your *pocket.", 18, 3, 16)} dy={30} blur={12} style={headStyle(72)} />
        <Words items={words("Now on iPhone.", 26, 2.4, 16)} dy={14} blur={8} style={{ ...subStyle(32), marginTop: 26 }} />
      </div>
    </AbsoluteFill>
  );
};

// ── 10. Lockup: every lamp of the mark ignites on the beat, the word slides out, the strip fills.
const WORD_RATIO = 4855 / 1157;
const WORD_OFFSET = 1606.1 / 1157;
const LockupShot = () => {
  const frame = useCurrentFrame();
  const size = 170;
  const lockW = size * (WORD_OFFSET + WORD_RATIO);
  const word = ramp(frame, 3, 18, EASE.snappy);
  const tag = ramp(frame, rel(SHOTS.lockup, 78), 20, EASE.out);
  const strip = ramp(frame, rel(SHOTS.lockup, 80), 26, EASE.smooth);
  const foot = ramp(frame, rel(SHOTS.lockup, 82), 20, EASE.out);
  const stamp = 1.06 - 0.06 * ramp(frame, 0, 8, EASE.out);
  const fade = ramp(frame, rel(SHOTS.lockup, 84), len(SHOTS.lockup) - rel(SHOTS.lockup, 84), EASE.smooth);
  const STRIP_W = 23 * 26 + 2 * 0.393 * 26;
  return (
    <AbsoluteFill style={{ opacity: 1 - fade }}>
      <div style={{ position: "absolute", left: 960 - lockW / 2, top: 300, transform: `scale(${stamp.toFixed(4)})`, transformOrigin: "50% 50%" }}>
        <div style={{ position: "relative", width: lockW, height: size }}>
          <Mark size={size} times={[0, 0, 1, 1, 1, 1, 2, 2, 2, 2]} />
          <Img
            src={staticFile("logo/word.svg")}
            style={{
              position: "absolute",
              left: size * WORD_OFFSET,
              top: 0,
              width: size * WORD_RATIO,
              height: size,
              opacity: word,
              transform: `translateX(${((1 - word) * -size * 0.3).toFixed(2)}px)`,
              filter: word > 0.999 ? undefined : `blur(${((1 - word) * 14).toFixed(2)}px)`,
            }}
          />
        </div>
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 540, textAlign: "center", opacity: tag, transform: `translateY(${((1 - tag) * 16).toFixed(2)}px)`, ...headStyle(54), fontStretch: "112%", fontWeight: 640, letterSpacing: "-0.02em" }}>
        Your Spotify stats, on your own server.
      </div>
      <div style={{ position: "absolute", left: 960 - STRIP_W / 2, top: 660, opacity: Math.min(1, strip * 3) }}>
        <Strip cols={24} pitch={26} seed={7} p={strip} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 830, textAlign: "center", opacity: foot, ...subStyle(30) }}>
        Free and open source · Desktop and iPhone
      </div>
      <Flash at={0} peak={0.5} dur={5} />
    </AbsoluteFill>
  );
};

// ── Sound. Each cue pins the file's own energy peak to its target frame (output offset measured 0 f).
const PEAK_MS: Record<string, number> = {
  "lamp-click-1": 30,
  "lamp-click-2": 40,
  "lamp-cascade": 790,
  hum: 0,
  selector: 0,
  knob: 340,
  "whoosh-1": 450,
  "whoosh-2": 570,
  "whoosh-long": 740,
  "glass-tap": 140,
  "tape-stop": 550,
  riser: 1990,
  impact: 20,
  shimmer: 660,
};
type Sfx = { from: number; src: string; volume: number };
const at = (target: number, src: string, volume: number): Sfx => ({
  from: Math.max(0, Math.round(target - (PEAK_MS[src] * 30) / 1000)),
  src,
  volume,
});
const alt = (i: number) => (i % 2 ? "lamp-click-2" : "lamp-click-1");

export const SFX: Sfx[] = [
  // Self-test: each meter sweeps on with a cascade; the camera hops between them.
  at(0, "hum", 0.55),
  ...[0, 1, 2, 3].map((i) => at(SWEEP(i) + 5, "lamp-cascade", 0.6)),
  ...[0, 1, 2, 3].map((i) => at(SWEEP(i), alt(i), 0.45)),
  ...[1, 2, 3].map((i) => at(SWEEP(i) - 5, "whoosh-2", 0.12)),
  // One word a beat: a hard relay click each, then the full line.
  ...WORDS.map((w, i) => at(B(w.beat), alt(i), 0.55)),
  at(B(12), "knob", 0.4),
  // Crane: the dashboard lights as it comes into view; riser into the drop.
  at(SHOTS.crane.from + 6, "lamp-cascade", 0.55),
  at(SHOTS.crane.from + CR.hold + 10, "lamp-cascade", 0.6),
  at(SHOTS.crane.from + CR.hold + 50, "lamp-cascade", 0.55),
  at(B(30) - 1, "riser", 0.38),
  // The drop: slam 1 of 3 and the train.
  at(B(30), "impact", 0.5),
  at(B(30) + 8, "whoosh-long", 0.4),
  // Top: crash zoom, slam 2 of 3.
  at(SHOTS.top.from + CRASH - 3, "whoosh-1", 0.4),
  at(SHOTS.top.from + CRASH, "impact", 0.38),
  // The selector clicks over on every page cut, landing on the beat.
  ...SELECTS.map((x) => at(x.at, "selector", 0.75)),
  // Artist: the chart climbs, the clock lights round the dial with a click a beat.
  at(SHOTS.artist.from + 8, "lamp-cascade", 0.6),
  ...[48, 49, 50, 51].map((b, i) => at(B(b), alt(i), 0.55)),
  // Sessions: the wave of session meters, a click a beat as rows light.
  at(SHOTS.sessions.from + 10, "lamp-cascade", 0.6),
  ...[55, 56, 57, 58, 59, 60].map((b, i) => at(B(b), alt(i), 0.45)),
  // Stats: columns climb.
  at(SHOTS.stats.from + 8, "lamp-cascade", 0.6),
  at(B(63) + 3, "impact", 0.3),
  // Pocket: the phone slides out; the music drops away.
  at(SHOTS.pocket.from + 6, "tape-stop", 0.4),
  at(SHOTS.pocket.from + 14, "whoosh-1", 0.35),
  at(SHOTS.pocket.from + 30, "glass-tap", 0.35),
  // Lockup: slam 3 of 3 as the mark ignites, then the strip fills and an afterglow.
  at(B(76), "impact", 0.5),
  ...[0, 1, 2].map((i) => at(B(76) + i, alt(i), 0.4)),
  at(B(80) + 14, "lamp-cascade", 0.35),
  at(B(78), "shimmer", 0.28),
];

const Bgm = () => {
  const frame = useCurrentFrame();
  const base = interpolate(frame, [0, 4, B(84), DURATION], [0, 0.72, 0.72, 0], clamp);
  const duck = SFX.filter((c) => c.src !== "tape-stop" && c.src !== "hum").reduce((d, c) => Math.max(d, interpolate(frame, [c.from - 4, c.from, c.from + 10, c.from + 18], [0, 1, 1, 0], clamp)), 0);
  return <Audio src={staticFile("audio/bgm-power.mp3")} volume={base * (1 - 0.4 * duck)} />;
};

const Scene = ({ s, children }: { s: Span; children: React.ReactNode }) => (
  <Sequence from={s.from} durationInFrames={len(s)}>
    {children}
  </Sequence>
);

export const Launch = ({ bgm = true }: { bgm?: boolean }) => (
  <AbsoluteFill style={{ background: "#0a0b0d", fontFamily: FONT }}>
    <Scene s={SHOTS.selftest}><SelfTest /></Scene>
    <Scene s={SHOTS.words}><WordBeats /></Scene>
    <Scene s={SHOTS.crane}><Crane /></Scene>
    <Scene s={SHOTS.wall}><Wall /></Scene>
    <Scene s={SHOTS.top}><TopShot /></Scene>
    <Scene s={SHOTS.artist}><ArtistShot /></Scene>
    <Scene s={SHOTS.sessions}><SessionsShot /></Scene>
    <Scene s={SHOTS.stats}><StatsShot /></Scene>
    <Scene s={SHOTS.pocket}><Pocket /></Scene>
    <Scene s={SHOTS.lockup}><LockupShot /></Scene>
    {SELECTS.map((x) => (
      <Sequence key={x.at} from={x.at - SEL} durationInFrames={SEL}>
        <Selector from={x.from} to={x.to} />
      </Sequence>
    ))}
    {bgm ? <Bgm /> : null}
    {SFX.map((s, i) => (
      <Sequence key={i} from={s.from}>
        <Audio src={staticFile(`audio/sfx/${s.src}.mp3`)} volume={s.volume} />
      </Sequence>
    ))}
  </AbsoluteFill>
);

