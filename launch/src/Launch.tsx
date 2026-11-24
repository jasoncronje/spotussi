// The spotussi iPhone launch video: 43 s at 30 fps, cut to bgm.mp3 (120 BPM).
// SHOTS is the single source of truth; the SFX table pins to it.
import type React from "react";
import { AbsoluteFill, Audio, Img, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";
import { C, EASE, beatF, ramp } from "./kit";
import { Caption, FloorShadow, LAMP_ORDER, Mark, Phone, PHONE_W, Strip, Words, headStyle, phoneH, subStyle, words } from "./parts";

const B = beatF;

export const SHOTS = {
  open: { from: 0, to: B(12) },
  hero: { from: B(12), to: B(20) },
  top: { from: B(20), to: B(26) },
  artist: { from: B(26), to: B(32) },
  sessions: { from: B(32), to: B(37) }, // the drop
  stats: { from: B(37), to: B(42) },
  library: { from: B(42), to: B(48) },
  wall: { from: B(48), to: B(56) }, // the breakdown
  outro: { from: B(56), to: 1290 },
} as const;
export const DURATION = SHOTS.outro.to;

const len = (s: { from: number; to: number }) => s.to - s.from;

// ── Lockup: the mark plus the outlined wordmark, at the brand's geometry.
const WORD_RATIO = 4855 / 1157; // word width per mark height
const WORD_OFFSET = 1606.1 / 1157; // gap from the mark's left edge to the word

const Lockup = ({ size, lampTimes, wordAt }: { size: number; lampTimes: number[]; wordAt: number }) => {
  const frame = useCurrentFrame();
  const p = ramp(frame, wordAt, 24, EASE.out);
  return (
    <div style={{ position: "relative", width: size * (WORD_OFFSET + WORD_RATIO), height: size }}>
      <Mark size={size} times={lampTimes} />
      <Img
        src={staticFile("logo/word.svg")}
        style={{
          position: "absolute",
          left: size * WORD_OFFSET,
          top: 0,
          width: size * WORD_RATIO,
          height: size,
          opacity: p,
          transform: `translateX(${((1 - p) * -size * 0.25).toFixed(2)}px)`,
          filter: p >= 0.999 ? undefined : `blur(${((1 - p) * 14).toFixed(2)}px)`,
        }}
      />
    </div>
  );
};

// ── 1. Open: the mark lights lamp by lamp on the off-beats, the word slides out of it.
const LAMP_TIMES = LAMP_ORDER.map((_, i) => B(1) + Math.round(i * 7.5));

const Open = () => {
  const frame = useCurrentFrame();
  const markSize = 200;
  const lockW = markSize * (WORD_OFFSET + WORD_RATIO);
  // The mark starts alone in the centre, then slides left as the word comes in.
  const slide = ramp(frame, B(6) - 6, 26, EASE.smooth);
  const x = 960 - markSize / 2 + (960 - lockW / 2 - (960 - markSize / 2)) * slide;
  const exit = ramp(frame, B(11.5), 12, EASE.exit);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit, transform: `translateY(${(-30 * exit).toFixed(2)}px)` }}>
      <div style={{ position: "absolute", left: x, top: 360 }}>
        <Lockup size={markSize} lampTimes={LAMP_TIMES} wordAt={B(6)} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 640, display: "flex", justifyContent: "center" }}>
        <Words items={words("Every play, kept *forever.", B(7.5))} dy={22} blur={10} style={{ ...headStyle(58), justifyContent: "center" }} />
      </div>
    </AbsoluteFill>
  );
};

// ── 2. Hero: the phone rises off the floor (element-body-moves contact-shadow-lift), caption on the left.
const PHONE_X = 1180;
const PHONE_Y = (1080 - phoneH(PHONE_W)) / 2;

const Hero = () => {
  const frame = useCurrentFrame();
  const rise = ramp(frame, 0, 34, EASE.out);
  const push = 1 + 0.025 * ramp(frame, 0, len(SHOTS.hero), EASE.smooth);
  const lift = 1 - rise; // 1 while it is still off the floor
  return (
    <AbsoluteFill>
      <FloorShadow x={PHONE_X + PHONE_W / 2} y={PHONE_Y + phoneH(PHONE_W)} w={PHONE_W * 0.9} lift={lift} />
      <Phone
        screen="overview"
        style={{
          left: PHONE_X,
          top: PHONE_Y,
          opacity: interpolate(rise, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
          transform: `translateY(${(lift * 120).toFixed(2)}px) scale(${(push * (0.96 + 0.04 * rise)).toFixed(4)})`,
        }}
      />
      <Caption lines={["Spotify stats,", "in your *pocket."]} sub="Your listening history, now on iPhone." at={10} />
    </AbsoluteFill>
  );
};

// ── 3–4. Feature shots: the phone glides in from the right, the old one has already left.
const Feature = ({
  screen,
  lines,
  sub,
  dur,
  enterFrom = 1,
  tilt = 0,
}: {
  screen: string;
  lines: string[];
  sub: string;
  dur: number;
  enterFrom?: 1 | -1;
  tilt?: number;
}) => {
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 18, EASE.snappy);
  const exit = ramp(frame, dur - 9, 9, EASE.exit);
  const drift = 1 + 0.02 * ramp(frame, 0, dur, EASE.smooth);
  const dx = (1 - enter) * 720 * enterFrom - exit * 900;
  const speed = Math.abs((1 - ramp(frame, 0, 18, (t) => t)) * 40) + exit * 50;
  return (
    <AbsoluteFill>
      <Phone
        screen={screen}
        style={{
          left: PHONE_X,
          top: PHONE_Y,
          transform: `translateX(${dx.toFixed(2)}px) rotate(${(tilt * enter).toFixed(2)}deg) scale(${drift.toFixed(4)})`,
          filter: speed > 1 ? `blur(${Math.min(10, speed * 0.2).toFixed(2)}px)` : undefined,
        }}
      />
      <div style={{ opacity: 1 - exit }}>
        <Caption lines={lines} sub={sub} at={6} />
      </div>
    </AbsoluteFill>
  );
};

// ── 5–7. The drop: three screens hit on the beat. The first hit has a single white flash.
const DropFeature = (props: { screen: string; lines: string[]; sub: string; dur: number; flash?: boolean }) => {
  const frame = useCurrentFrame();
  const settle = 1.035 - 0.035 * ramp(frame, 0, 8, EASE.out); // shutter afterglow
  const flash = props.flash ? interpolate(frame, [0, 4], [0.9, 0], { extrapolateRight: "clamp", easing: EASE.out }) : 0;
  const exit = ramp(frame, props.dur - 6, 6, EASE.exit);
  return (
    <AbsoluteFill>
      <AbsoluteFill style={{ transform: `scale(${settle.toFixed(4)})`, opacity: 1 - exit }}>
        <Phone screen={props.screen} style={{ left: PHONE_X, top: PHONE_Y }} />
        <Caption lines={props.lines} sub={props.sub} at={2} />
      </AbsoluteFill>
      {flash > 0 ? <AbsoluteFill style={{ background: C.paper, opacity: flash }} /> : null}
    </AbsoluteFill>
  );
};

// ── 8. Breakdown: what you get, one lamp per line.
const WALL = ["Live capture", "Lifetime history", "Every artist, album, track", "Works offline", "Free and open source"];
export const WALL_AT = (i: number) => 16 + i * 15; // one line per beat, relative to the shot

const Wall = () => {
  const frame = useCurrentFrame();
  const exit = ramp(frame, len(SHOTS.wall) - 10, 10, EASE.exit);
  return (
    <AbsoluteFill style={{ opacity: 1 - exit }}>
      <div style={{ position: "absolute", left: 168, top: 200 }}>
        <Words items={words("All yours.", 0)} dy={30} blur={14} style={headStyle(92)} />
        <Words items={words("On *your server.", 10)} dy={30} blur={14} style={headStyle(92)} />
      </div>
      <div style={{ position: "absolute", left: 1010, top: 196, width: 760 }}>
        {WALL.map((t, i) => {
          const p = ramp(frame, WALL_AT(i), 14, EASE.out);
          const lamp = interpolate(frame, [WALL_AT(i) + 4, WALL_AT(i) + 6], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
          return (
            <div
              key={t}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 28,
                padding: "26px 0",
                borderTop: `2px solid ${C.grille}`,
                opacity: p,
                transform: `translateY(${((1 - p) * 18).toFixed(2)}px)`,
                ...headStyle(44),
                fontStretch: "112%",
                fontWeight: 640,
                letterSpacing: "-0.02em",
              }}
            >
              <span style={{ width: 22, height: 22, borderRadius: "50%", background: C.grille, position: "relative", flex: "none" }}>
                <span style={{ position: "absolute", inset: 0, borderRadius: "50%", background: C.lamp, opacity: lamp }} />
              </span>
              {t}
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
};

// ── 9. Outro: every screen the film showed comes back for the group photo
// (outro-group-photo-launch), then the lockup lands and the meter strip fills.
const GROUP = [
  { screen: "library", x: 120, rot: -9, dx: -520, cue: 0 },
  { screen: "stats", x: 470, rot: -4, dx: -380, cue: 7 },
  { screen: "overview", x: 820, rot: 0, dx: 0, cue: 15 },
  { screen: "artist", x: 1170, rot: 4, dx: 380, cue: 22 },
  { screen: "sessions", x: 1520, rot: 9, dx: 520, cue: 30 },
];
export const OUTRO_LOCK = B(60) - B(56); // the lockup stamp, relative to the shot
const GW = 380;

const Outro = () => {
  const frame = useCurrentFrame();
  const crane = ramp(frame, 0, 40, EASE.out);
  const push = 0.04 * ramp(frame, 40, 300, EASE.smooth);
  const recede = ramp(frame, OUTRO_LOCK - 8, 10, EASE.out);
  const lock = ramp(frame, OUTRO_LOCK, 22, EASE.out);
  const tag = ramp(frame, OUTRO_LOCK + 16, 22, EASE.out);
  const strip = ramp(frame, OUTRO_LOCK + 20, 30, EASE.smooth);
  const fade = ramp(frame, B(76) - B(56), len(SHOTS.outro) - (B(76) - B(56)), EASE.smooth);
  return (
    <AbsoluteFill style={{ opacity: 1 - fade }}>
      <AbsoluteFill
        style={{
          transform: `perspective(1400px) rotateX(${(4 - 4 * crane).toFixed(3)}deg) scale(${(1.06 - 0.06 * crane + push).toFixed(4)})`,
        }}
      >
        {GROUP.map((g) => {
          const t = ramp(frame, g.cue, 14, EASE.fly);
          if (frame < g.cue) return null;
          return (
            <Phone
              key={g.screen}
              screen={g.screen}
              w={GW}
              style={{
                left: g.x,
                top: 500 + Math.abs(g.rot) * 6,
                opacity: interpolate(frame, [g.cue, g.cue + 3], [0, 1], { extrapolateRight: "clamp" }) * (1 - 0.15 * recede),
                transform: `translate(${((1 - t) * g.dx).toFixed(2)}px, ${((1 - t) * 420).toFixed(2)}px) rotate(${(g.rot * (2 - t)).toFixed(2)}deg)`,
                filter: `saturate(${(1 - 0.2 * recede).toFixed(3)})`,
              }}
            />
          );
        })}
        {/* The ground darkens toward the bottom so the lockup reads above the phones. */}
        <AbsoluteFill style={{ background: `linear-gradient(to bottom, ${C.console} 0%, rgba(18,19,23,${(0.4 * recede).toFixed(3)}) 50%, rgba(18,19,23,0) 100%)` }} />
      </AbsoluteFill>
      <div
        style={{
          position: "absolute",
          left: 960 - (120 * (WORD_OFFSET + WORD_RATIO)) / 2,
          top: 170,
          opacity: lock,
          transform: `translateY(${((1 - lock) * 28).toFixed(2)}px) scale(${(1.04 - 0.04 * lock).toFixed(4)})`,
          filter: lock >= 0.999 ? undefined : `blur(${((1 - lock) * 10).toFixed(2)}px)`,
        }}
      >
        <Lockup size={120} lampTimes={LAMP_ORDER.map(() => -100)} wordAt={-100} />
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 336, textAlign: "center", opacity: tag, transform: `translateY(${((1 - tag) * 14).toFixed(2)}px)`, ...subStyle(38), color: C.paper }}>
        Spotify stats, self-hosted.
      </div>
      <div style={{ position: "absolute", left: 960 - 15 * 24 * 0.5, top: 420 }}>
        <Strip cols={16} pitch={24} seed={5} p={strip} />
      </div>
    </AbsoluteFill>
  );
};

// ── Sound. Each entry pins the sound's own peak to a moment in a shot:
// from = target − the file's peak − the output track's AAC offset (sound-design §4.6).
// Measured 2026-11-23 (Remotion 4.0.536, AAC 48 kHz MP4): two impact probes cross-correlated
// against draft 1 both started exactly on their Sequence frame, so this pipeline adds no lag.
const OUTPUT_AUDIO_OFFSET_F = 0;
const PEAK_MS: Record<string, number> = {
  "lamp-click-1": 30,
  "lamp-click-2": 40,
  "lamp-cascade": 790,
  knob: 1340,
  "whoosh-1": 450,
  "whoosh-2": 570,
  "whoosh-long": 740,
  "glass-tap": 140,
  "tape-stop": 550,
  riser: 1990,
  impact: 20,
  shimmer: 660,
};
type Sfx = { from: number; src: string; volume: number; durationInFrames?: number };
const at = (target: number, src: string, volume: number, durationInFrames?: number): Sfx => ({
  from: Math.max(0, Math.round(target - (PEAK_MS[src] * 30) / 1000 - OUTPUT_AUDIO_OFFSET_F)),
  src,
  volume,
  durationInFrames,
});

export const SFX: Sfx[] = [
  // Open: one relay click per lamp, two takes alternating, stepping down so it doesn't read as a machine gun.
  ...LAMP_TIMES.map((f, i) => at(f, i % 2 ? "lamp-click-2" : "lamp-click-1", 0.55 - i * 0.02)),
  at(B(6) + 8, "whoosh-2", 0.25), // the word slides out of the mark
  // Hero: the phone rises.
  at(SHOTS.hero.from + 10, "whoosh-1", 0.35),
  // Features glide in.
  at(SHOTS.top.from + 6, "whoosh-2", 0.4),
  at(SHOTS.artist.from + 6, "whoosh-1", 0.4),
  // Build into the drop, then the first slam.
  at(SHOTS.sessions.from - 1, "riser", 0.45),
  at(SHOTS.sessions.from, "impact", 0.45),
  at(SHOTS.stats.from + 2, "knob", 0.35, 30), // range switch feel on Stats
  at(SHOTS.library.from, "glass-tap", 0.7),
  at(SHOTS.library.to - 2, "tape-stop", 0.4), // the drop ends, the breakdown starts
  // Breakdown: a lamp per line.
  ...[0, 1, 2, 3, 4].map((i) => at(SHOTS.wall.from + WALL_AT(i) + 4, i % 2 ? "lamp-click-2" : "lamp-click-1", 0.5)),
  // Outro: phones arrive, the lockup stamps (second and last slam), the strip fills, afterglow.
  at(SHOTS.outro.from + 10, "whoosh-long", 0.35),
  at(SHOTS.outro.from + OUTRO_LOCK, "impact", 0.42),
  at(SHOTS.outro.from + OUTRO_LOCK + 34, "lamp-cascade", 0.45),
  at(SHOTS.outro.from + OUTRO_LOCK + 30, "shimmer", 0.22),
];

const Bgm = () => {
  const frame = useCurrentFrame();
  const v = interpolate(frame, [0, 6, B(76), DURATION], [0, 0.6, 0.6, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return <Audio src={staticFile("audio/bgm.mp3")} volume={v} />;
};

const Scene = ({ s, children }: { s: { from: number; to: number }; children: React.ReactNode }) => (
  <Sequence from={s.from} durationInFrames={len(s)}>
    {children}
  </Sequence>
);

export const Launch = ({ bgm = true }: { bgm?: boolean }) => (
  <AbsoluteFill style={{ background: C.console }}>
    <Scene s={SHOTS.open}><Open /></Scene>
    <Scene s={SHOTS.hero}><Hero /></Scene>
    <Scene s={SHOTS.top}>
      <Feature screen="top" lines={["Know your", "*real top 10."]} sub="Tracks, artists, albums and genres, ranked by plays or time." dur={len(SHOTS.top)} />
    </Scene>
    <Scene s={SHOTS.artist}>
      <Feature screen="artist" lines={["Go deep on", "any *artist."]} sub="Rank, all-time plays and your first listen." dur={len(SHOTS.artist)} tilt={-3} />
    </Scene>
    <Scene s={SHOTS.sessions}>
      <DropFeature screen="sessions" lines={["Every session,", "*remembered."]} sub="Grouped into the stretches you actually listened." dur={len(SHOTS.sessions)} flash />
    </Scene>
    <Scene s={SHOTS.stats}>
      <DropFeature screen="stats" lines={["Watch your", "taste *move."]} sub="Plays by day and artist share, week on week." dur={len(SHOTS.stats)} />
    </Scene>
    <Scene s={SHOTS.library}>
      <DropFeature screen="library" lines={["Find the songs", "you *never play."]} sub="Saved, unplayed and how long songs take to stick." dur={len(SHOTS.library)} />
    </Scene>
    <Scene s={SHOTS.wall}><Wall /></Scene>
    <Scene s={SHOTS.outro}><Outro /></Scene>
    {bgm ? <Bgm /> : null}
    {SFX.map((s, i) => (
      <Sequence key={i} from={s.from} durationInFrames={s.durationInFrames}>
        <Audio src={staticFile(`audio/sfx/${s.src}.mp3`)} volume={s.volume} />
      </Sequence>
    ))}
  </AbsoluteFill>
);
