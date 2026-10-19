// The frame around the page: a flat app window around the desktop layout, or
// a phone around the phone layout. Drawn in page space, above and around the
// captured viewport, so the window and the page share one set of coordinates.
import type React from "react";
import { Interactive } from "remotion";
import { FONT } from "./Figures";

export const STAGE = "#0b0c0f";
const EDGE = "#2b2d34";
const BAR = "#1a1b20";

type WindowProps = {
  readonly kind: "desktop" | "phone";
  /** Page width and the height of the screen the window shows. */
  readonly w: number;
  readonly h: number;
  readonly style?: React.CSSProperties;
};

const WindowInner = ({ kind, w, h, style }: WindowProps) =>
  kind === "desktop" ? (
    <div style={{ position: "absolute", inset: 0, ...style }}>
      {/* Title bar: three window controls and the address of your server. */}
      <div
        style={{
          position: "absolute",
          left: 0,
          top: -44,
          width: w,
          height: 44,
          background: BAR,
          borderRadius: "12px 12px 0 0",
        }}
      >
        <div
          style={{
            position: "absolute",
            left: 18,
            top: 16,
            display: "flex",
            gap: 8,
          }}
        >
          {[0, 1, 2].map((i) => (
            <span
              key={i}
              style={{
                width: 12,
                height: 12,
                borderRadius: 6,
                background: "#3a3d46",
              }}
            />
          ))}
        </div>
        <div
          style={{
            position: "absolute",
            left: w / 2 - 150,
            top: 9,
            width: 300,
            height: 26,
            borderRadius: 7,
            background: STAGE,
            color: "#9a9ea8",
            fontFamily: FONT,
            fontSize: 13,
            lineHeight: "26px",
            textAlign: "center",
          }}
        >
          spotussi.local
        </div>
      </div>
      <div
        style={{
          position: "absolute",
          left: -1.5,
          top: -45.5,
          width: w + 3,
          height: h + 47,
          border: `1.5px solid ${EDGE}`,
          borderRadius: 13.5,
          boxSizing: "border-box",
        }}
      />
    </div>
  ) : (
    <div style={{ position: "absolute", inset: 0, ...style }}>
      {/* Status bar over the top of the screen, with the camera island. */}
      <div
        style={{
          position: "absolute",
          left: 0,
          top: -47,
          width: w,
          height: 47,
          background: "#0f1013",
          borderRadius: "42px 42px 0 0",
          color: "#eeeff1",
          fontFamily: FONT,
          fontSize: 16,
          fontWeight: 600,
        }}
      >
        <span style={{ position: "absolute", left: 40, top: 15 }}>9:41</span>
        <span
          style={{
            position: "absolute",
            left: w / 2 - 62,
            top: 11,
            width: 124,
            height: 34,
            borderRadius: 17,
            background: "#000",
          }}
        />
        <div
          style={{
            position: "absolute",
            right: 34,
            top: 19,
            display: "flex",
            gap: 6,
            alignItems: "flex-end",
          }}
        >
          {[5, 7, 9, 11].map((bar) => (
            <span
              key={bar}
              style={{
                width: 3,
                height: bar,
                borderRadius: 1,
                background: "#eeeff1",
              }}
            />
          ))}
          <span
            style={{
              width: 24,
              height: 11,
              marginLeft: 4,
              borderRadius: 3,
              border: "1.5px solid #9a9ea8",
              boxSizing: "border-box",
              padding: 1.5,
            }}
          >
            <span
              style={{
                display: "block",
                width: "80%",
                height: "100%",
                borderRadius: 1,
                background: "#eeeff1",
              }}
            />
          </span>
        </div>
      </div>
      {/* The phone's body. */}
      <div
        style={{
          position: "absolute",
          left: -12,
          top: -59,
          width: w + 24,
          height: h + 71,
          border: `12px solid ${BAR}`,
          borderRadius: 54,
          boxSizing: "border-box",
        }}
      />
    </div>
  );

export const Window = Interactive.create({
  Component: WindowInner,
  layout: "none",
  schema: {} as const,
});
