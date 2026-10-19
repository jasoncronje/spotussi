// The tour: someone using the app. The window never moves. A pointer (a
// cursor on the desktop, a fingertip on the phone) clicks the app's own nav
// to go from page to page, and the top artist's row to open their page; the
// page swaps the moment it is clicked, its body zooms and rises into place
// under the app's header, and it powers on, its lamps lighting one by one. On the phone each page scrolls once, by a thumb, down to the part the
// caption is about. Times inside a shot are seconds from the shot's start. The
// site's caption times in site/main.js follow `beats`, so change them together.
import type React from "react";
import { Easing, interpolate } from "remotion";
import { Chart, Lamps } from "./Lamps";
import { layouts, type NavLabel, type Stop } from "./page";

export const LENGTH = 21;
/** How long each page holds before the next click. */
export const PAGE = 3.5;
/** How long the screen takes to swap one page for the next after a click. */
export const FADE = 0.36;

export type Framing = "wide" | "phone";
export type Point = { x: number; y: number };

/** The screen the window shows, in page pixels (the capture's viewport). */
export const screens = {
  wide: { h: 900, radius: 12 },
  phone: { h: 844, radius: 42 },
};
/**
 * Where the frame looks, held for the whole reel: the window-space point at
 * its centre and the width in view. The wide cut holds the whole window; the
 * phone cut holds the top of the phone's screen at full width.
 */
export const views = {
  wide: { x: 720, y: 428, w: 1600 },
  phone: { x: 195, y: 243.75, w: 390 },
};
/** How much of the phone's screen the 4:5 frame shows, from the top. */
export const PHONE_VIEW = (views.phone.w * 1350) / 1080;

export type Shot = {
  readonly stop: Stop;
  /** When it is clicked and the screen swaps to it (seconds from the start of the loop). */
  readonly at: number;
  /** When the summary figures count up; without it they show their value. */
  readonly count?: [number, number];
  /** The page's lamps, lighting at their times (frames, via `f`). */
  readonly lights?: (f: (s: number) => number) => React.ReactNode;
  /** What is clicked to leave for the next shot, in window space. */
  readonly click?: Point;
};

/** The caption beats on the site: when each page takes over. */
export const beats: Record<Stop, number> = {
  overview: 0,
  top: PAGE,
  artist: PAGE * 2,
  history: PAGE * 3,
  sessions: PAGE * 4,
  stats: PAGE * 5,
};
/** Overview is clicked and returns dark here, so the loop joins without a cut. */
export const HOME = 20.2;

/** The phone's one scroll per page: when, how far, and its easing. */
export const SCROLL: [number, number] = [0.45, 1.5];
const scrollEase = Easing.bezier(0.4, 0, 0.2, 1);
const depth: Record<Framing, Record<Stop, number>> = {
  wide: { overview: 0, top: 0, artist: 0, history: 0, sessions: 0, stats: 0 },
  phone: {
    overview: 330,
    top: 0,
    artist: 520,
    history: 520,
    sessions: 160,
    stats: 130,
  },
};
export const scrollAt = (
  framing: Framing,
  stop: Stop,
  t: number,
  /** The dark Overview the loop ends on stays at the top, like the frame it joins. */
  last = false,
) =>
  interpolate(t, SCROLL, [0, last ? 0 : depth[framing][stop]], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: scrollEase,
  });
/** How long a page's contents take to zoom into place after the click that opens it. */
export const SETTLE = 0.6;
/** The app's own chrome at the top of each page (header and range bar), which holds still while the body moves. */
export const CHROME: Record<Framing, number> = { wide: 99, phone: 106 };
export type Motion = { scale: number; dy: number; ox: number; oy: number };
/**
 * How a page's body moves once it is clicked open: it zooms in from a little
 * smaller and rises into place, centred on the screen. The window, the app's
 * header and range bar and the phone's tab bar stay put. `ox`/`oy` is the
 * origin, in page pixels.
 */
export const contentAt = (
  framing: Framing,
  stop: Stop,
  t: number,
  entering: boolean,
): Motion => {
  const e = entering
    ? interpolate(t, [FADE / 2, FADE / 2 + SETTLE], [0, 1], {
        extrapolateLeft: "clamp",
        extrapolateRight: "clamp",
        easing: Easing.out(Easing.cubic),
      })
    : 1;
  const mid = framing === "wide" ? screens.wide.h / 2 : PHONE_VIEW / 2;
  return {
    scale: 1 - 0.06 * (1 - e),
    dy: 24 * (1 - e),
    ox: layouts[framing][stop].page.w / 2,
    oy: depth[framing][stop] + mid,
  };
};
/** Where a page point shows on screen at `t` (seconds into the shot). */
const onScreen = (framing: Framing, stop: Stop, p: Point, t: number): Point => {
  const m = contentAt(framing, stop, t, true);
  return {
    x: m.ox + (p.x - m.ox) * m.scale,
    y: m.oy + (p.y - m.oy) * m.scale + m.dy - scrollAt(framing, stop, t),
  };
};

/** Lamps wait for the page to settle after a scroll, so they light in view. */
const lag = (framing: Framing, stop: Stop) =>
  depth[framing][stop] > 0 ? 0.9 : 0;

type Lights = (f: (s: number) => number) => React.ReactNode;

const overview =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].overview;
    const g = (s: number) => f(s + lag(framing, "overview"));
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.6} from={f(0.2)} />
        {l.meters.map((m, i) => (
          <Lamps
            key={i}
            name={`Meter ${i + 1}`}
            lamps={m}
            spread={0.9}
            settle
            from={g(0.5 + i * 0.05)}
          />
        ))}
        <Chart name="Plays by day" stagger={0.03} climb={0.45} from={g(0.9)} />
        <Lamps
          name="Listening clock"
          lamps={l.clockLamps}
          spread={1.0}
          from={g(1.2)}
        />
        <Lamps name="Rank bars" lamps={l.rankBars} spread={0.7} from={g(1.6)} />
        <Lamps name="Glyphs" lamps={l.dots} spread={0.7} from={g(1.7)} />
      </>
    );
  };

const top =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].top;
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.3} from={f(0.2)} />
        <Lamps name="Rank bars" lamps={l.rankBars} spread={0.9} from={f(0.35)} />
      </>
    );
  };

const artist =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].artist;
    const g = (s: number) => f(s + lag(framing, "artist"));
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.3} from={f(0.2)} />
        <Chart name="Plays by day" stagger={0.045} climb={0.5} from={g(0.6)} />
        <Lamps
          name="Listening clock"
          lamps={l.clockLamps}
          spread={0.9}
          from={g(0.9)}
        />
        <Lamps name="Rank bars" lamps={l.rankBars} spread={0.7} from={g(1.3)} />
        <Lamps name="Glyphs" lamps={l.dots} spread={0.7} from={g(1.3)} />
      </>
    );
  };

const history =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].history;
    const g = (s: number) => f(s + lag(framing, "history"));
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.3} from={f(0.2)} />
        {/* Each row's source lamps, top to bottom. */}
        <Lamps
          name="Source lamps"
          lamps={l.dots}
          spread={1}
          wave={{ x: 0.25, y: 0.7 }}
          from={g(0.35)}
        />
      </>
    );
  };

const sessions =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].sessions;
    const g = (s: number) => f(s + lag(framing, "sessions"));
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.3} from={f(0.2)} />
        {/* Every session's dot train runs left to right, the longest first. */}
        <Lamps
          name="Dot trains"
          lamps={l.dots}
          spread={1}
          wave={framing === "wide" ? { x: 1.1, y: 0.4 } : { x: 2, y: 0.4 }}
          from={g(0.35)}
        />
      </>
    );
  };

const stats =
  (framing: Framing): Lights =>
  (f) => {
    const l = layouts[framing].stats;
    const g = (s: number) => f(s + lag(framing, "stats"));
    return (
      <>
        <Lamps name="Pilot lamps" lamps={l.pilots} spread={0.3} from={f(0.2)} />
        <Chart name="Plays by day" stagger={0.03} climb={0.45} from={g(0.35)} />
      </>
    );
  };

/**
 * Where the pointer clicks to open a page: a nav link (the header's on the
 * desktop, the tab bar's at the foot of the frame on the phone), or the top
 * artist's name in the Top artists list.
 */
const navAt = (framing: Framing, from: Stop, label: NavLabel): Point => {
  const l = layouts[framing][from];
  const link = l.links[label];
  // The header holds still, so its links are where the capture found them.
  if (framing === "wide")
    return { x: link.x + link.w / 2, y: link.y + link.h / 2 + 2 };
  // The tab bar is fixed to the foot of the frame: it never moves.
  const bar = l.tabbar!;
  return {
    x: link.x + link.w / 2,
    y: PHONE_VIEW - bar.h + (link.y - bar.y) + link.h / 2 - 6,
  };
};
const rowAt = (framing: Framing, t: number): Point => {
  const row = layouts[framing].top.firstRow!;
  const p =
    framing === "wide"
      ? { x: row.x + 170, y: row.y + row.h / 2 }
      : { x: row.x + 150, y: row.y + row.h / 2 };
  return onScreen(framing, "top", p, t);
};

const tour = (framing: Framing): Shot[] => {
  const lagged = (stop: Stop, [a, b]: [number, number]): [number, number] => [
    a + lag(framing, stop),
    b + lag(framing, stop),
  ];
  return [
    {
      stop: "overview",
      at: 0,
      count: lagged("overview", [0.5, 1.6]),
      lights: overview(framing),
      click: navAt(framing, "overview", "Top"),
    },
    {
      stop: "top",
      at: beats.top,
      lights: top(framing),
      click: rowAt(framing, PAGE),
    },
    {
      stop: "artist",
      at: beats.artist,
      count: lagged("artist", [0.35, 1.2]),
      lights: artist(framing),
      click: navAt(framing, "artist", "History"),
    },
    {
      stop: "history",
      at: beats.history,
      lights: history(framing),
      click: navAt(framing, "history", "Sessions"),
    },
    {
      stop: "sessions",
      at: beats.sessions,
      lights: sessions(framing),
      click: navAt(framing, "sessions", "Stats"),
    },
    {
      stop: "stats",
      at: beats.stats,
      lights: stats(framing),
      click: navAt(framing, "stats", "Overview"),
    },
    // Dark, its figures at zero: the frame the loop opens on.
    { stop: "overview", at: HOME, count: [LENGTH, LENGTH + 1] },
  ];
};

export const tours: Record<Framing, Shot[]> = {
  wide: tour("wide"),
  phone: tour("phone"),
};
