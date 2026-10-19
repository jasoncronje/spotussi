// The hand in the tour: what makes each page change. On the desktop a cursor
// glides to what it clicks, presses, and the page swaps; it rests where it
// clicked until the next move. On the phone a fingertip touches down: it drags
// each page up once, and taps the tab bar (or the top artist's row).
import { Easing, interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import {
  type Framing,
  HOME,
  type Point,
  SCROLL,
  scrollAt,
  tours,
  views,
} from "./Tour";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
const glide = Easing.bezier(0.45, 0, 0.25, 1);

/** Each click: when it lands (the next page's start) and where. */
const clicksOf = (framing: Framing) => {
  const tour = tours[framing];
  return tour.flatMap((s, i) =>
    s.click ? [{ at: tour[i + 1].at, p: s.click }] : [],
  );
};

/** How long the cursor takes to reach what it clicks, and the pause before it presses. */
const MOVE = 0.65;
const HOVER = 0.35;

const cursorAt = (framing: Framing, t: number): Point => {
  const clicks = clicksOf(framing);
  // At the start of the loop it rests where the last click (Overview) left it.
  let p = clicks[clicks.length - 1].p;
  for (const c of clicks) {
    const start = c.at - HOVER - MOVE;
    if (t < start) break;
    const u = interpolate(t, [start, c.at - HOVER], [0, 1], {
      ...clamp,
      easing: glide,
    });
    // A slight arc, as a hand moves.
    const dx = c.p.x - p.x;
    const dy = c.p.y - p.y;
    const bow = Math.sin(Math.PI * u) * 0.08;
    p = { x: p.x + dx * u - dy * bow, y: p.y + dy * u + dx * bow };
  }
  return p;
};

/** The press and the ring it leaves, around a click at `at`. */
const press = (t: number, at: number) => ({
  squeeze: interpolate(t, [at - 0.1, at, at + 0.12], [1, 0.86, 1], clamp),
  ring: interpolate(t, [at, at + 0.45], [0, 1], {
    ...clamp,
    easing: Easing.out(Easing.quad),
  }),
});

const Ring = ({ p, ring, r }: { p: Point; ring: number; r: number }) =>
  ring > 0 && ring < 1 ? (
    <circle
      cx={p.x}
      cy={p.y}
      r={4 + ring * r}
      fill="none"
      stroke="rgba(255,255,255,0.75)"
      strokeWidth={2}
      opacity={1 - ring}
    />
  ) : null;

const Cursor = ({ t }: { t: number }) => {
  const p = cursorAt("wide", t);
  const near = clicksOf("wide").find((c) => Math.abs(t - c.at) < 0.6);
  const { squeeze, ring } = near ? press(t, near.at) : { squeeze: 1, ring: 0 };
  return (
    <>
      {near ? <Ring p={near.p} ring={ring} r={34} /> : null}
      {/* An arrow, its tip on the point; 41 page pixels tall, so it reads at the size the site shows the reel. */}
      <g
        transform={`translate(${p.x} ${p.y}) scale(${1.8 * squeeze})`}
        style={{ filter: "drop-shadow(0 2px 3px rgba(0,0,0,0.5))" }}
      >
        <path
          d="M0 0 L0 20 L4.8 15.6 L8 22.8 L11.2 21.4 L8.1 14.4 L14.4 14.4 Z"
          fill="#ffffff"
          stroke="#111216"
          strokeWidth={1.2}
          strokeLinejoin="round"
        />
      </g>
    </>
  );
};

const Fingertip = ({
  p,
  shown,
  squeeze = 1,
}: {
  p: Point;
  shown: number;
  squeeze?: number;
}) =>
  shown > 0 ? (
    <circle
      cx={p.x}
      cy={p.y}
      r={19 * squeeze * (1.15 - 0.15 * shown)}
      fill="rgba(255,255,255,0.22)"
      stroke="rgba(255,255,255,0.6)"
      strokeWidth={1.5}
      opacity={shown}
    />
  ) : null;

/** How far a thumb drags before it lets the page run on by itself. */
const DRAG = 230;

const Touches = ({ t, fps }: { t: number; fps: number }) => {
  const tour = tours.phone;
  // The shot on screen now.
  const shot = tour.filter((s) => t >= s.at).pop()!;
  const local = t - shot.at;
  const out: React.ReactNode[] = [];

  // The drag: the fingertip moves with the page until it lifts.
  const full = scrollAt("phone", shot.stop, SCROLL[1], shot.at === HOME);
  if (full > 0) {
    const reach = Math.min(full, DRAG);
    let lift = SCROLL[1];
    for (let s = SCROLL[0]; s <= SCROLL[1]; s += 1 / fps)
      if (scrollAt("phone", shot.stop, s) >= reach) {
        lift = s;
        break;
      }
    const y0 = 400;
    const moved = Math.min(scrollAt("phone", shot.stop, local), reach);
    const shown =
      interpolate(local, [SCROLL[0] - 0.15, SCROLL[0]], [0, 1], clamp) *
      interpolate(local, [lift, lift + 0.18], [1, 0], clamp);
    out.push(
      <Fingertip key="drag" p={{ x: 292, y: y0 - moved }} shown={shown} />,
    );
  }

  // The tap that opens the next page, and its ring once the page has swapped.
  for (const c of clicksOf("phone")) {
    if (t < c.at - 0.45 || t > c.at + 0.45) continue;
    const { squeeze, ring } = press(t, c.at);
    const shown =
      interpolate(t, [c.at - 0.45, c.at - 0.3], [0, 1], clamp) *
      interpolate(t, [c.at + 0.05, c.at + 0.3], [1, 0], clamp);
    out.push(
      <Ring key={`ring-${c.at}`} p={c.p} ring={ring} r={30} />,
      <Fingertip key={`tap-${c.at}`} p={c.p} shown={shown} squeeze={squeeze} />,
    );
  }
  return <>{out}</>;
};

export const Pointer = ({ framing }: { framing: Framing }) => {
  const frame = useCurrentFrame();
  const { fps, width, height } = useVideoConfig();
  const t = frame / fps;
  const view = views[framing];
  const scale = width / view.w;
  const left = view.x - view.w / 2;
  const top = view.y - (view.w * height) / width / 2;
  return (
    <svg
      width={width}
      height={height}
      viewBox={`${left} ${top} ${width / scale} ${height / scale}`}
      style={{ position: "absolute", inset: 0, overflow: "visible" }}
    >
      {framing === "wide" ? <Cursor t={t} /> : <Touches t={t} fps={fps} />}
    </svg>
  );
};
