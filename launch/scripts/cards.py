"""Cut the UI cards the promo uses out of the hero reel's lit desktop captures.

    python3 launch/scripts/cards.py   (from the repo root; needs ffmpeg)

Reads launch/src/cards.json (page-pixel rects) and motion/public/1440/<stop>-lit.png
(2.5x captures), writes launch/public/cards/<id>.png. The output is not committed.
"""
import json, pathlib, subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
cards = json.loads((ROOT / "launch/src/cards.json").read_text())
out = ROOT / "launch/public/cards"
out.mkdir(parents=True, exist_ok=True)
S = 2.5
for name, c in cards.items():
    crop = f"crop={int(c['w']*S)}:{int(c['h']*S)}:{int(c['x']*S)}:{int(c['y']*S)}"
    src = ROOT / f"motion/public/1440/{c['stop']}-lit.png"
    subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-i", str(src), "-vf", crop, str(out / f"{name}.png")], check=True)
print(f"{len(cards)} cards written to {out}")
