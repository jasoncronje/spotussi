#!/bin/sh
# Renders the hero reel into the site: wide and phone, H.264 MP4 (no WebM: VP9
# decodes the dark ground darker than the page and is no smaller), and a
# poster of each: the Overview lit (3 s, before the fingertip comes to tap
# the next page).
set -e
cd "$(dirname "$0")/.."
out=../site/assets
for c in HeroReel HeroReelPhone; do
  n=hero-reel
  [ "$c" = HeroReelPhone ] && n=hero-reel-phone
  npx remotion render "$c" "$out/$n.mp4" --codec=h264 --crf=24 --pixel-format=yuv420p
  npx remotion still "$c" "out/$n-poster.png" --frame=90
  cwebp -quiet -q 82 "out/$n-poster.png" -o "$out/$n-poster.webp"
done
