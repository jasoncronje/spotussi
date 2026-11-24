import { Composition } from "remotion";
import { DURATION, Launch } from "./Launch";

export const RemotionRoot = () => (
  <Composition
    id="Launch"
    component={Launch}
    durationInFrames={DURATION}
    fps={30}
    width={1920}
    height={1080}
    defaultProps={{ bgm: true }}
  />
);
