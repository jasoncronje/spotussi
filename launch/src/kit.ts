// Shared timing, easing and brand tokens for the launch video.
import { loadFont } from "@remotion/fonts";
import { Easing, staticFile } from "remotion";

export const FONT = "Archivo Variable";
loadFont({
  family: FONT,
  url: staticFile("archivo-wdth.woff2"),
  weight: "100 900",
  stretch: "62% 125%",
});

export const C = {
  console: "#121317",
  graphite: "#1A1C21",
  grille: "#3A3D46",
  lamp: "#1DB954",
  paper: "#ECEEF1",
  mist: "#A1A5AC",
};

// bgm.mp3 is 120 BPM: one beat is 15 frames, the first beat sits at 0.526 s.
export const BEAT0 = 15.78;
export const beatF = (n: number) => Math.round(BEAT0 + 15 * n);

// Brand motion voice: professional and instrument-like. Entrances settle
// without bounce; only things that land (phones in the outro) overshoot.
export const EASE = {
  out: Easing.bezier(0.22, 1, 0.36, 1),
  snappy: Easing.bezier(0.16, 1, 0.3, 1),
  smooth: Easing.bezier(0.65, 0, 0.35, 1),
  exit: Easing.bezier(0.7, 0, 0.84, 0),
  fly: Easing.bezier(0.34, 1.4, 0.44, 1),
};

const clamp01 = (t: number) => Math.min(1, Math.max(0, t));
export const ramp = (
  frame: number,
  start: number,
  dur: number,
  ease: (t: number) => number = EASE.out,
) => ease(clamp01((frame - start) / Math.max(1e-6, dur)));

/** Seeded random, so every render is identical. */
export const mulberry32 = (seed: number) => () => {
  let t = (seed += 0x6d2b79f5);
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};
