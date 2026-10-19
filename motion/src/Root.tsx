import { Composition } from "remotion";
import { LENGTH, Reel } from "./Reel";

// The site's hero frame is 3:2 on desktop (the app window's shape) and 4:5 on
// phones; 21 s at 30 fps.
export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="HeroReel"
        component={Reel}
        durationInFrames={LENGTH * 30}
        fps={30}
        width={1800}
        height={1200}
        defaultProps={{
          framing: "wide",
          style: { backgroundColor: "#0b0c0f" },
        }}
      />
      <Composition
        id="HeroReelPhone"
        component={Reel}
        durationInFrames={LENGTH * 30}
        fps={30}
        width={1080}
        height={1350}
        defaultProps={{
          framing: "phone",
          style: { backgroundColor: "#0b0c0f" },
        }}
      />
    </>
  );
};
