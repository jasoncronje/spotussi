// Lamps are cut from the lit capture and laid over the dark one, so every lamp
// that lights is the app's own pixels. Each lights with a short incandescent
// flash, the way a meter lamp overshoots before it settles.
import type React from "react";
import {
  Easing,
  Img,
  Interactive,
  interpolate,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { useLayout, type Lamp, type Rect } from "./page";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

/** One piece of the lit capture, placed where it sits on the page. */
export const Sprite = ({
  rect,
  style,
}: {
  readonly rect: Rect;
  readonly style?: React.CSSProperties;
}) => {
  const { lit, page } = useLayout();
  const pad = 1;
  return (
    <div
      style={{
        position: "absolute",
        left: rect.x - pad,
        top: rect.y - pad,
        width: rect.w + pad * 2,
        height: rect.h + pad * 2,
        overflow: "hidden",
        ...style,
      }}
    >
      <Img
        src={lit}
        style={{
          position: "absolute",
          left: -(rect.x - pad),
          top: -(rect.y - pad),
          width: page.w,
          height: page.h,
          maxWidth: "none",
        }}
      />
    </div>
  );
};

/** Opacity and flash for a lamp that lights at frame `at`. */
const ignite = (frame: number, at: number): React.CSSProperties => ({
  opacity: interpolate(frame, [at, at + 2], [0, 1], clamp),
  filter: `brightness(${interpolate(frame, [at, at + 2, at + 9], [2.1, 2.1, 1], clamp)})`,
});

type LampsProps = {
  readonly lamps: Lamp[];
  /** Seconds from the first lamp to the last. */
  readonly spread: number;
  /** Ease-out timing: lamps come fast, then slow, like a needle settling. */
  readonly settle?: boolean;
  /**
   * Light by position instead of order: seconds per 1000 page pixels across
   * and down, so a wave runs over the page (`spread` is then ignored).
   */
  readonly wave?: { x: number; y: number };
  readonly style?: React.CSSProperties;
};

const LampsInner = ({
  lamps,
  spread,
  settle = false,
  wave,
  style,
}: LampsProps) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const n = Math.max(1, lamps.length - 1);
  const x0 = Math.min(...lamps.map((l) => l.x));
  const y0 = Math.min(...lamps.map((l) => l.y));
  return (
    <div style={{ position: "absolute", inset: 0, ...style }}>
      {lamps.map((l, i) => {
        const t = settle ? 1 - Math.sqrt(1 - i / n) : i / n;
        const at = wave
          ? (((l.x - x0) * wave.x + (l.y - y0) * wave.y) / 1000) * fps
          : t * spread * fps;
        // Rank bars sweep in from the left instead of flashing.
        if (l.kind === "bar") {
          const p = interpolate(frame, [at, at + 0.55 * fps], [0, 100], {
            ...clamp,
            easing: Easing.bezier(0.16, 1, 0.3, 1),
          });
          return (
            <Sprite
              key={i}
              rect={l}
              style={{ clipPath: `inset(0 ${100 - p}% 0 0)` }}
            />
          );
        }
        return <Sprite key={i} rect={l} style={ignite(frame, at)} />;
      })}
    </div>
  );
};

export const Lamps = Interactive.create({
  Component: LampsInner,
  layout: "none",
  schema: {
    spread: {
      type: "number",
      default: 1,
      min: 0.1,
      step: 0.1,
      description: "Seconds from first lamp to last",
      hiddenFromList: false,
    },
  } as const,
});

type ChartProps = {
  /** Seconds between neighbouring columns starting. */
  readonly stagger: number;
  /** Seconds for one column to climb to its value. */
  readonly climb: number;
  readonly style?: React.CSSProperties;
};

/** Plays by day: each column climbs lamp by lamp from the baseline. */
const ChartInner = ({ stagger, climb, style }: ChartProps) => {
  const { columns } = useLayout();
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  return (
    <div style={{ position: "absolute", inset: 0, ...style }}>
      {columns.map((c, i) => {
        const height = c.base - c.top;
        const count = Math.ceil(height / c.pitch);
        const start = i * stagger * fps;
        const p = interpolate(
          frame,
          [start, start + climb * fps * Math.sqrt(count / 30)],
          [0, 1],
          {
            ...clamp,
            easing: Easing.bezier(0.3, 0, 0.2, 1),
          },
        );
        const lit = Math.round(p * count);
        if (lit === 0) return null;
        const shown = Math.min(height, lit * c.pitch);
        const rect = { x: c.x, y: c.top, w: c.w, h: height };
        const cap = {
          x: c.x,
          y: Math.max(c.top, c.base - lit * c.pitch),
          w: c.w,
          h: c.pitch,
        };
        return (
          <div key={i}>
            <Sprite
              rect={rect}
              style={{ clipPath: `inset(${height - shown + 1}px 0 0 0)` }}
            />
            {/* The lamp at the top of a climbing column burns hot. */}
            <Sprite
              rect={cap}
              style={{
                filter: "brightness(2.1)",
                opacity: interpolate(p, [0.9, 1], [1, 0], clamp),
              }}
            />
          </div>
        );
      })}
    </div>
  );
};

export const Chart = Interactive.create({
  Component: ChartInner,
  layout: "none",
  schema: {
    stagger: {
      type: "number",
      default: 0.05,
      min: 0,
      step: 0.01,
      description: "Seconds between columns",
      hiddenFromList: false,
    },
    climb: {
      type: "number",
      default: 0.6,
      min: 0.1,
      step: 0.05,
      description: "Seconds for a column to climb",
      hiddenFromList: false,
    },
  } as const,
});
