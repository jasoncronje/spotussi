// The summary figures, set in the app's own type (Archivo, wide cut) exactly
// where the app draws them, counting from zero as their meters light.
import { loadFont } from "@remotion/fonts";
import type React from "react";
import { Interactive, staticFile } from "remotion";
import { useLayout } from "./page";

export const FONT = "Archivo Variable";
loadFont({
  family: FONT,
  url: staticFile("archivo-wdth.woff2"),
  weight: "100 900",
  stretch: "62% 125%",
});

const format = (target: string, p: number) => {
  const n = Number(target.replace(/,/g, ""));
  // A rank (a single digit) is shown as it is rather than counted.
  if (!Number.isFinite(n) || n < 10) return target;
  return Math.round(n * p).toLocaleString("en-US");
};

type FiguresProps = {
  /** 0 to 1: how far each figure has counted. */
  readonly progress: number;
  readonly style?: React.CSSProperties;
};

const FiguresInner = ({ progress, style }: FiguresProps) => {
  const { figures } = useLayout();
  return (
    <div style={{ position: "absolute", inset: 0, ...style }}>
      {figures.map((f, i) => (
        <p
          key={i}
          style={{
            position: "absolute",
            left: f.x,
            top: f.y,
            margin: 0,
            fontFamily: FONT,
            fontSize: f.font.size,
            fontWeight: Number(f.font.weight),
            fontStretch: f.font.stretch,
            letterSpacing: f.font.spacing,
            color: f.font.color,
            fontFeatureSettings: f.font.features,
            lineHeight: 1,
            whiteSpace: "nowrap",
          }}
        >
          {f.spans.map((s, j) => (
            <span
              key={j}
              style={{
                fontSize: s.size,
                fontWeight: Number(s.weight),
                color: s.color,
                letterSpacing: s.spacing,
                marginLeft: s.marginLeft,
              }}
            >
              {/\d/.test(s.text) ? format(s.text, progress) : s.text}
            </span>
          ))}
        </p>
      ))}
    </div>
  );
};

export const Figures = Interactive.create({
  Component: FiguresInner,
  layout: "none",
  schema: {
    progress: {
      type: "number",
      default: 1,
      min: 0,
      max: 1,
      step: 0.01,
      description: "Count progress",
      hiddenFromList: false,
    },
  } as const,
});
