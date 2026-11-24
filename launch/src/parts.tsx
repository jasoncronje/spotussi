// Building blocks: blur-slide words, the drawn phone, the mark and the meter strip.
import type React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { C, EASE, FONT, mulberry32, ramp } from "./kit";

// ── blur-slide (video-shotcraft typography/blur-slide): y, blur and opacity
// share one progress and one ease-out, so each word rises out of soft focus.
export type Word = { text: string; start: number; dur?: number; lit?: boolean };

export const words = (text: string, start: number, gap = 4, dur = 22): Word[] =>
  text.split(" ").map((t, i) => {
    const lit = t.startsWith("*");
    return { text: lit ? t.slice(1) : t, start: start + i * gap, dur, lit };
  });

export const Words = ({
  items,
  dy,
  blur,
  style,
}: {
  readonly items: Word[];
  readonly dy: number;
  readonly blur: number;
  readonly style: React.CSSProperties;
}) => {
  const frame = useCurrentFrame();
  return (
    <div style={{ display: "flex", flexWrap: "wrap", columnGap: "0.26em", ...style }}>
      {items.map((w, i) => {
        const p = ramp(frame, w.start, w.dur ?? 22, EASE.out);
        return (
          <span
            key={i}
            style={{
              display: "inline-block",
              opacity: p,
              transform: `translateY(${((1 - p) * dy).toFixed(2)}px)`,
              filter: p >= 0.999 ? undefined : `blur(${((1 - p) * blur).toFixed(2)}px)`,
              color: w.lit ? C.lamp : undefined,
            }}
          >
            {w.text}
          </span>
        );
      })}
    </div>
  );
};

export const headStyle = (size: number): React.CSSProperties => ({
  fontFamily: FONT,
  fontStretch: "125%",
  fontWeight: 760,
  letterSpacing: "-0.035em",
  lineHeight: 1.02,
  fontSize: size,
  color: C.paper,
});

export const subStyle = (size: number): React.CSSProperties => ({
  fontFamily: FONT,
  fontWeight: 420,
  fontSize: size,
  lineHeight: 1.35,
  color: C.mist,
});

/** A headline (two lines, "*word" is lit) and a sub line, staggered so they read as one sentence. */
export const Caption = ({
  lines,
  sub,
  at,
  x = 168,
  y = 360,
  size = 92,
}: {
  readonly lines: string[];
  readonly sub?: string;
  readonly at: number;
  readonly x?: number;
  readonly y?: number;
  readonly size?: number;
}) => {
  let t = at;
  const rows = lines.map((l) => {
    const w = words(l, t);
    t += w.length * 4 + 4;
    return w;
  });
  return (
    <div style={{ position: "absolute", left: x, top: y, width: 900 }}>
      {rows.map((r, i) => (
        <Words key={i} items={r} dy={size * 0.38} blur={16} style={headStyle(size)} />
      ))}
      {sub ? (
        <Words
          items={words(sub, at + 14, 2.4, 20)}
          dy={20}
          blur={8}
          style={{ ...subStyle(size * 0.36), marginTop: size * 0.42, columnGap: "0.28em" }}
        />
      ) : null}
    </div>
  );
};

// ── The drawn phone, the same build as brand/scripts/marketing.mjs.
export const PHONE_W = 440;
export const phoneH = (w: number) => {
  const pad = w * 0.028;
  return ((w - 2 * pad) * 2622) / 1206 + 2 * pad;
};

export const Phone = ({
  screen,
  w = PHONE_W,
  style,
}: {
  readonly screen: string;
  readonly w?: number;
  readonly style?: React.CSSProperties;
}) => {
  const pad = w * 0.028;
  const sw = w - 2 * pad;
  const sh = (sw * 2622) / 1206;
  const r = w * 0.15;
  return (
    <div
      style={{
        position: "absolute",
        width: w,
        height: sh + 2 * pad,
        padding: pad,
        borderRadius: r,
        background: "#0B0C0E",
        boxShadow: `0 0 0 ${Math.max(1.5, w * 0.0068).toFixed(1)}px ${C.grille}`,
        ...style,
      }}
    >
      <Img
        src={staticFile(`screens/${screen}.png`)}
        style={{ display: "block", width: sw, height: sh, borderRadius: r - pad }}
      />
      <i
        style={{
          position: "absolute",
          left: "50%",
          transform: "translateX(-50%)",
          top: pad + sw * 0.028,
          width: sw * 0.3,
          height: sw * 0.088,
          background: "#000",
          borderRadius: 999,
        }}
      />
    </div>
  );
};

/** Soft floor shadow under a phone, separate from the body so it can stay behind as the phone lifts. */
export const FloorShadow = ({ x, y, w, lift }: { x: number; y: number; w: number; lift: number }) => (
  <div
    style={{
      position: "absolute",
      left: x - w * (0.5 + 0.2 * lift),
      top: y - 22,
      width: w * (1 + 0.4 * lift),
      height: 44 + 30 * lift,
      borderRadius: "50%",
      background: "radial-gradient(closest-side, rgba(0,0,0,.65), rgba(0,0,0,0))",
      opacity: 0.85 - 0.45 * lift,
      filter: `blur(${(10 + 22 * lift).toFixed(1)}px)`,
    }}
  />
);

// ── The mark: a 4×4 grille lit 2·4·3·1 from the bottom. `lit[i]` is the
// frame lamp i lights at (column by column, bottom up); unset lamps stay dark.
export const LIT = [2, 4, 3, 1];
export const LAMP_ORDER: [number, number][] = LIT.flatMap((h, c) =>
  Array.from({ length: h }, (_, k) => [c, 3 - k] as [number, number]),
);

const ignite = (frame: number, at: number) => ({
  on: interpolate(frame, [at, at + 2], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
  flash: interpolate(frame, [at, at + 2, at + 9], [1.9, 1.9, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
});

export const Mark = ({ size, times }: { size: number; times: number[] }) => {
  const frame = useCurrentFrame();
  // Brand geometry: lamp radius is 0.393 of the pitch, so the mark is 3.786 pitches wide.
  const pitch = size / 3.786;
  const r = pitch * 0.393;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ display: "block", overflow: "visible" }}>
      {[0, 1, 2, 3].flatMap((c) =>
        [0, 1, 2, 3].map((row) => {
          const i = LAMP_ORDER.findIndex(([lc, lr]) => lc === c && lr === row);
          const cx = r + c * pitch;
          const cy = r + row * pitch;
          const g = i >= 0 && times[i] !== undefined ? ignite(frame, times[i]) : { on: 0, flash: 1 };
          return (
            <g key={`${c}-${row}`}>
              <circle cx={cx} cy={cy} r={r} fill={C.grille} />
              <circle cx={cx} cy={cy} r={r} fill={C.lamp} opacity={g.on} style={{ filter: `brightness(${g.flash})` }} />
            </g>
          );
        }),
      )}
    </svg>
  );
};

/** Meter strip: the grille run on. Heights are a seeded wave, never real data. `p` lights it left to right. */
export const Strip = ({ cols, pitch, seed, p }: { cols: number; pitch: number; seed: number; p: number }) => {
  const rnd = mulberry32(seed);
  const r = pitch * 0.393;
  const w = (cols - 1) * pitch + 2 * r;
  const h = 3 * pitch + 2 * r;
  const lamps: React.ReactNode[] = [];
  for (let c = 0; c < cols; c++) {
    const v = 1.5 + 1.35 * Math.sin(c * 0.31 + seed) + 0.75 * Math.sin(c * 0.97 + seed * 2) + (c % 7 >= 5 ? 0.6 : 0) + (rnd() - 0.5) * 0.9;
    const hgt = Math.max(1, Math.min(4, Math.round(v)));
    const colOn = Math.min(1, Math.max(0, p * cols - c));
    for (let row = 0; row < 4; row++) {
      const lit = 3 - row < hgt;
      lamps.push(
        <g key={`${c}-${row}`}>
          <circle cx={r + c * pitch} cy={r + row * pitch} r={r} fill={C.grille} />
          {lit ? <circle cx={r + c * pitch} cy={r + row * pitch} r={r} fill={C.lamp} opacity={colOn} /> : null}
        </g>,
      );
    }
  }
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} style={{ display: "block" }}>
      {lamps}
    </svg>
  );
};
