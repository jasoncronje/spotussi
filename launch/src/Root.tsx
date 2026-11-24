import { Composition } from "remotion";
import { DURATION, Launch } from "./Power";
import { DURATION as V1_DURATION, Launch as LaunchV1 } from "./LaunchV1";

export const RemotionRoot = () => (
  <>
    <Composition id="Launch" component={Launch} durationInFrames={DURATION} fps={30} width={1920} height={1080} defaultProps={{ bgm: true }} />
    {/* Draft 1, kept for reference: the phone-first cut. */}
    <Composition id="LaunchV1" component={LaunchV1} durationInFrames={V1_DURATION} fps={30} width={1920} height={1080} defaultProps={{ bgm: true }} />
  </>
);
