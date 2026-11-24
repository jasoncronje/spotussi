import { Composition } from "remotion";
import { DURATION, Launch } from "./Promo";
import { DURATION as V2_DURATION, Launch as LaunchV2 } from "./Power";
import { DURATION as V1_DURATION, Launch as LaunchV1 } from "./LaunchV1";

export const RemotionRoot = () => (
  <>
    <Composition id="Launch" component={Launch} durationInFrames={DURATION} fps={30} width={1920} height={1080} defaultProps={{ bgm: true }} />
    {/* Earlier drafts, kept for reference. */}
    <Composition id="LaunchV2" component={LaunchV2} durationInFrames={V2_DURATION} fps={30} width={1920} height={1080} defaultProps={{ bgm: true }} />
    <Composition id="LaunchV1" component={LaunchV1} durationInFrames={V1_DURATION} fps={30} width={1920} height={1080} defaultProps={{ bgm: true }} />
  </>
);
