"""Outline the spotussi wordmark from Archivo so the logo files need no font.

    uv run --with fonttools --with brotli --with uharfbuzz \
        python brand/scripts/outline.py <archivo-latin-wdth-normal.woff2> [wght] [wdth] [tracking] [text] \
        > brand/src/wordmark.json

Shapes the text (default "spotussi") with HarfBuzz (kerning included) at the given instance,
applies tracking in thousandths of an em, and prints one SVG path in font
units (1000/em) with the baseline at y=0 and y pointing down, plus the
advance width and x-height. The woff2 ships with @fontsource-variable/archivo
(web/node_modules/@fontsource-variable/archivo/files/).
"""
import io
import json
import sys

import uharfbuzz as hb
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont

src = sys.argv[1]
wght = float(sys.argv[2]) if len(sys.argv) > 2 else 760
wdth = float(sys.argv[3]) if len(sys.argv) > 3 else 125
tracking = float(sys.argv[4]) if len(sys.argv) > 4 else -35
text = sys.argv[5] if len(sys.argv) > 5 else "spotussi"

font = instantiateVariableFont(TTFont(src), {"wght": wght, "wdth": wdth})
font.flavor = None
buf = io.BytesIO()
font.save(buf)
blob = hb.Blob(buf.getvalue())
hbfont = hb.Font(hb.Face(blob))
b = hb.Buffer()
b.add_str(text)
b.guess_segment_properties()
hb.shape(hbfont, b, {"kern": True, "liga": False})

glyphs = font.getGlyphSet()
order = font.getGlyphOrder()
pen = SVGPathPen(glyphs, ntos=lambda v: f"{v:.1f}".rstrip("0").rstrip("."))
x = 0.0
n = len(b.glyph_infos)
for i, (info, pos) in enumerate(zip(b.glyph_infos, b.glyph_positions)):
    name = order[info.codepoint]
    glyphs[name].draw(TransformPen(pen, (1, 0, 0, -1, x + pos.x_offset, -pos.y_offset)))
    x += pos.x_advance + (tracking if i < n - 1 else 0)

print(json.dumps({"d": pen.getCommands(), "advance": round(x, 1),
                  "xHeight": font["OS/2"].sxHeight, "ascender": font["hhea"].ascent,
                  "wght": wght, "wdth": wdth, "tracking": tracking}))
