// The captured app pages (scripts/capture.mjs), one set per app layout, and
// their lamps grouped by the part of the page they belong to. Coordinates are
// CSS pixels of the captured page.
import { createContext, useContext } from "react";
import { staticFile } from "remotion";
import wideArtist from "./captures/1440-artist.json";
import wideHistory from "./captures/1440-history.json";
import wideOverview from "./captures/1440-overview.json";
import wideSessions from "./captures/1440-sessions.json";
import wideStats from "./captures/1440-stats.json";
import wideTop from "./captures/1440-top.json";
import phoneArtist from "./captures/390-artist.json";
import phoneHistory from "./captures/390-history.json";
import phoneOverview from "./captures/390-overview.json";
import phoneSessions from "./captures/390-sessions.json";
import phoneStats from "./captures/390-stats.json";
import phoneTop from "./captures/390-top.json";

export type Rect = { x: number; y: number; w: number; h: number };
export type Lamp = Rect & { kind: "dot" | "bar"; clock: boolean };
export type Column = {
  x: number;
  base: number;
  top: number;
  w: number;
  pitch: number;
};
export type Figure = Rect & {
  spans: {
    text: string;
    size: string;
    weight: string;
    color: string;
    spacing: string;
    marginLeft: string;
  }[];
  font: {
    size: string;
    weight: string;
    stretch: string;
    spacing: string;
    color: string;
    features: string;
  };
};
type Panel = { heading: Rect; panel: Rect };
/** A nav link: the header's on the desktop, the tab bar's (fixed) on the phone. */
type Link = Rect & { label: string; href: string; fixed: boolean };
type Capture = {
  page: { w: number; h: number };
  tabbar: Rect | null;
  firstRow: Rect | null;
  nav: Link[];
  rangebar: Rect | null;
  figures: Figure[];
  lamps: Lamp[];
  bars: Column[];
  clock: Rect | null;
  headings: Record<string, Panel>;
};

export const stops = [
  "overview",
  "top",
  "artist",
  "history",
  "sessions",
  "stats",
] as const;
export type Stop = (typeof stops)[number];

/** The app's own pages, as its nav names them. */
export const NAV = ["Overview", "Top", "Stats", "History", "Sessions"] as const;
export type NavLabel = (typeof NAV)[number];

const byX = (a: Rect, b: Rect) => a.x - b.x;
const byRow = (a: Rect, b: Rect) => a.y - b.y || a.x - b.x;

/** The range bar under the app header: its lamps light first on every page. */
const TOOLBAR = 120;

const layout = (stop: Stop, data: Capture) => {
  const { page, figures, lamps } = data;

  // The level meter under each summary figure, left to right; it ends where
  // the next figure in the same row begins. Only the Overview has them.
  const meters =
    stop === "overview"
      ? figures.map((f) => {
          const next = Math.min(
            ...figures.filter((g) => g.y === f.y && g.x > f.x).map((g) => g.x),
            f.x + 300,
          );
          return lamps
            .filter(
              (l) =>
                l.kind === "dot" &&
                !l.clock &&
                l.y > f.y &&
                l.y < f.y + f.h + 40 &&
                l.x > f.x - 4 &&
                l.x < next - 4,
            )
            .sort(byX);
        })
      : [];
  const inMeter = new Set(meters.flat());

  // The listening clock: clockwise from midnight, inner lamp first.
  const clock = data.clock;
  const clockLamps = clock
    ? lamps
        .filter((l) => l.clock)
        .map((l) => {
          const dx = l.x + l.w / 2 - (clock.x + clock.w / 2);
          const dy = l.y + l.h / 2 - (clock.y + clock.h / 2);
          return {
            ...l,
            turn: (Math.atan2(dx, -dy) / (2 * Math.PI) + 1) % 1,
            radius: Math.hypot(dx, dy),
          };
        })
        .sort((a, b) => a.turn - b.turn || a.radius - b.radius)
    : [];

  const dots = lamps.filter(
    (l) => l.kind === "dot" && !l.clock && !inMeter.has(l),
  );
  // On the Overview the pilots are every small lamp above the plays-by-day
  // chart; elsewhere, the range bar's.
  const chartTop = data.headings["Plays by day"]?.panel.y ?? TOOLBAR;
  const pilotLine = stop === "overview" ? chartTop : TOOLBAR;

  return {
    stop,
    page,
    lit: staticFile(`${page.w}/${stop}-lit.png`),
    dark: staticFile(`${page.w}/${stop}-dark.png`),
    tabbar: data.tabbar
      ? { ...data.tabbar, src: staticFile(`${page.w}/${stop}-tabbar.png`) }
      : null,
    /** The first row of a list: on Top artists, the artist the reel opens. */
    firstRow: data.firstRow,
    /** Where each nav link sits: in page pixels on the desktop, in the tab bar's own pixels on the phone. */
    links: Object.fromEntries(
      data.nav
        .filter(
          (l) =>
            (NAV as readonly string[]).includes(l.label) &&
            l.fixed === (page.w < 600),
        )
        .map((l) => [l.label, l]),
    ) as Record<NavLabel, Link>,
    /** The phone's range bar, which sticks to the top of the screen once the header scrolls away. */
    rangebar: data.rangebar
      ? { ...data.rangebar, src: staticFile(`${page.w}/${stop}-rangebar.png`) }
      : null,
    figures,
    columns: data.bars,
    meters,
    /** Small lamps near the top that light as the page powers on. */
    pilots: dots.filter((l) => l.y < pilotLine).sort(byX),
    /** Every other small lamp, row by row. */
    dots: dots.filter((l) => l.y >= pilotLine).sort(byRow),
    clockLamps,
    /** Rank and share bars, row by row. */
    rankBars: lamps.filter((l) => l.kind === "bar").sort(byRow),
  };
};

export type Layout = ReturnType<typeof layout>;
type Pages = Record<Stop, Layout>;
const set = (captures: Record<Stop, unknown>) =>
  Object.fromEntries(
    stops.map((s) => [s, layout(s, captures[s] as Capture)]),
  ) as Pages;

export const layouts: Record<"wide" | "phone", Pages> = {
  wide: set({
    overview: wideOverview,
    top: wideTop,
    artist: wideArtist,
    history: wideHistory,
    sessions: wideSessions,
    stats: wideStats,
  }),
  phone: set({
    overview: phoneOverview,
    top: phoneTop,
    artist: phoneArtist,
    history: phoneHistory,
    sessions: phoneSessions,
    stats: phoneStats,
  }),
};
export const LayoutContext = createContext<Layout>(layouts.wide.overview);
export const useLayout = () => useContext(LayoutContext);
