// The hero reel: someone using the app (src/Tour.tsx). The window holds
// still; a pointer (src/Pointer.tsx) clicks from page to page, each page swaps
// in dark and powers on, and on the phone a thumb scrolls each page once.
//
// The wide cut is the desktop layout in a flat app window, the whole window in
// frame. The phone cut is the app's own phone layout at the frame's full
// width, its tab bar pinned to the bottom of the frame.
import {
  Easing,
  Img,
  Interactive,
  interpolate,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { Figures } from "./Figures";
import { LayoutContext, layouts } from "./page";
import { Pointer } from "./Pointer";
import {
  CHROME,
  contentAt,
  FADE,
  HOME,
  LENGTH,
  scrollAt,
  screens,
  type Shot,
  tours,
  views,
} from "./Tour";
import { STAGE, Window } from "./Window";

export { LENGTH, STAGE };
const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

type ShotProps = {
  readonly shot: Shot;
  readonly framing: "wide" | "phone";
  /** Seconds into the shot. */
  readonly t: number;
  /** How much of the page shows: it dims out, then the next dims in. */
  readonly p: number;
  /** 0 to 1 as the page's tab bar fades in over the last one. */
  readonly bar: number;
  /** The world (window and page), or the phone's tab bar, drawn over every world. */
  readonly layer: "world" | "tabbar";
};

const ShotView = ({ shot, framing, t, p, bar, layer }: ShotProps) => {
  const { fps, width, height } = useVideoConfig();
  const layout = layouts[framing][shot.stop];
  const { page, tabbar } = layout;
  const screen = screens[framing];
  const view = views[framing];
  const scale = width / view.w;
  const left = view.x - view.w / 2;
  const top = view.y - (view.w * height) / width / 2;
  const scroll = scrollAt(framing, shot.stop, t, shot.at === HOME);
  const { rangebar } = layout;
  const motion = contentAt(framing, shot.stop, t, shot.at > 0);
  const count = shot.count
    ? interpolate(t, shot.count, [0, 1], {
        ...clamp,
        easing: Easing.out(Easing.quad),
      })
    : 1;
  const f = (s: number) => Math.round((shot.at + s) * fps);

  if (layer === "tabbar")
    return tabbar ? (
      <Img
        src={tabbar.src}
        style={{
          position: "absolute",
          left: -left * scale,
          top: Math.min(height, (screen.h - top) * scale) - tabbar.h * scale,
          width: page.w * scale,
          height: tabbar.h * scale,
          opacity: bar,
        }}
      />
    ) : null;

  return (
    <LayoutContext.Provider value={layout}>
      <div
        style={{
          position: "absolute",
          left: 0,
          top: 0,
          width: page.w,
          height: screen.h,
          transformOrigin: "0 0",
          translate: `${-scale * left}px ${-scale * top}px`,
          scale: String(scale),
        }}
      >
        {/* The screen: the page is cut off at its edges. */}
        <div
          style={{
            position: "absolute",
            inset: 0,
            overflow: "hidden",
            opacity: p,
            borderRadius: `0 0 ${screen.radius}px ${screen.radius}px`,
          }}
        >
          {/* The page twice: its chrome (header and range bar), which holds
              still, and its body, which zooms into place under it. */}
          {(["chrome", "body"] as const).map((part) => (
            <div
              key={part}
              style={{
                position: "absolute",
                left: 0,
                top: -scroll,
                width: page.w,
                height: page.h,
                clipPath:
                  part === "chrome"
                    ? `inset(0 0 ${page.h - CHROME[framing]}px 0)`
                    : `inset(${CHROME[framing]}px 0 0 0)`,
              }}
            >
              <div
                style={{
                  position: "absolute",
                  inset: 0,
                  background: STAGE,
                  ...(part === "body"
                    ? {
                        // Clipped in its own space too, so its copy of the chrome never shows.
                        clipPath: `inset(${CHROME[framing]}px 0 0 0)`,
                        transformOrigin: `${motion.ox}px ${motion.oy}px`,
                        transform: `translateY(${motion.dy}px) scale(${motion.scale})`,
                      }
                    : {}),
                }}
              >
                <Img
                  src={layout.dark}
                  style={{
                    position: "absolute",
                    inset: 0,
                    width: page.w,
                    height: page.h,
                  }}
                />
                {shot.lights?.(f)}
                <Figures name="Summary figures" progress={count} />
              </div>
            </div>
          ))}
          {/* Once the header scrolls away the range bar sticks to the top. */}
          {rangebar && scroll > rangebar.y ? (
            <Img
              src={rangebar.src}
              style={{
                position: "absolute",
                left: rangebar.x,
                top: 0,
                width: rangebar.w,
                height: rangebar.h,
              }}
            />
          ) : null}
        </div>
        <Window
          name="Window"
          kind={framing === "wide" ? "desktop" : "phone"}
          w={page.w}
          h={screen.h}
        />
      </div>
    </LayoutContext.Provider>
  );
};

type ReelProps = {
  readonly framing: "wide" | "phone";
};

const ReelInner = ({ framing }: ReelProps) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const tour = tours[framing];
  const t = frame / fps;

  // A click swaps the page: it dims out over the first half of the change and
  // the next dims in over the second, so they never overlap. The window and
  // its chrome stay as they are.
  const half = FADE / 2;
  const fade = { ...clamp, easing: Easing.inOut(Easing.quad) };
  const shown = tour.flatMap((shot, i) => {
    const next = tour[i + 1];
    const end = next ? next.at + FADE : LENGTH;
    if (t < shot.at || t >= end) return [];
    const into =
      shot.at === 0
        ? 1
        : interpolate(t, [shot.at + half, shot.at + FADE], [0, 1], fade);
    const out = next
      ? interpolate(t, [next.at, next.at + half], [1, 0], fade)
      : 1;
    const bar =
      shot.at === 0
        ? 1
        : interpolate(t, [shot.at, shot.at + FADE], [0, 1], fade);
    return [
      {
        key: i,
        props: { shot, framing, t: t - shot.at, p: Math.min(into, out), bar },
      },
    ];
  });

  return (
    <>
      {shown.map(({ key, props }) => (
        <ShotView key={key} layer="world" {...props} />
      ))}
      {shown.map(({ key, props }) => (
        <ShotView key={`tabbar-${key}`} layer="tabbar" {...props} />
      ))}
      <Pointer framing={framing} />
    </>
  );
};

export const Reel = Interactive.create({
  Component: ReelInner,
  layout: "absolute-fill",
  schema: {
    ...Interactive.backgroundSchema,
    framing: {
      type: "enum",
      default: "wide",
      description: "Framing",
      variants: { wide: {}, phone: {} },
    },
  } as const,
});
