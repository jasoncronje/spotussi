#!/usr/bin/env python3
"""Builds the static iPhone mockup sheet (no JavaScript, no external assets)."""
import math
import random
import sys

random.seed(7)

# ---------- small helpers ----------

def art(initials, hue, size=44, radius=10, cls=""):
    return (
        f'<span class="art {cls}" style="width:{size}px;height:{size}px;border-radius:{radius}px;'
        f'background:linear-gradient(135deg,hsl({hue} 70% 52%),hsl({(hue + 48) % 360} 75% 34%));'
        f'font-size:{max(10, size // 3)}px">{initials}</span>'
    )


ICONS = {
    "gauge": '<path d="M4 15a8 8 0 1 1 16 0"/><path d="M12 15l4-5"/>',
    "list": '<path d="M9 6h11M9 12h11M9 18h11"/><path d="M4 5v3M4 11h1v3M3.5 17h1.5v3h-1.5"/>',
    "chart": '<path d="M5 20V10M12 20V4M19 20v-7"/>',
    "clock": '<circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/>',
    "search": '<circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/>',
    "gear": '<circle cx="12" cy="12" r="3"/><path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1"/>',
    "back": '<path d="M15 5l-7 7 7 7"/>',
    "more": '<circle cx="5" cy="12" r="1.4" fill="currentColor"/><circle cx="12" cy="12" r="1.4" fill="currentColor"/><circle cx="19" cy="12" r="1.4" fill="currentColor"/>',
    "chev": '<path d="M7 10l5 5 5-5"/>',
    "qr": '<path d="M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4z"/><path d="M14 14h2v2h-2zM18 14h2v2M14 18h2v2M18 18h2v2"/>',
    "filter": '<path d="M4 7h16M7 12h10M10 17h4"/>',
    "lib": '<path d="M5 4v16M10 4v16"/><path d="M14.5 5.5l4.5 14"/>',
    "share": '<path d="M12 15V4M8 8l4-4 4 4"/><path d="M6 12v7h12v-7"/>',
}


def icon(name, size=22, stroke=1.9):
    return (
        f'<svg class="ic" width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" '
        f'stroke="currentColor" stroke-width="{stroke}" stroke-linecap="round" stroke-linejoin="round">'
        f"{ICONS[name]}</svg>"
    )


def status_bar():
    return (
        '<div class="status"><span class="time">9:41</span><span class="island"></span>'
        '<span class="sys"><i class="bars"><b></b><b></b><b></b><b></b></i>'
        '<i class="batt"><b></b></i></span></div>'
    )


def tabbar(active, accessory=True):
    tabs = [("gauge", "Overview"), ("list", "Top"), ("chart", "Stats"), ("clock", "History"), ("lib", "Library")]
    items = "".join(
        f'<span class="tab{" on" if label == active else ""}">{icon(ic, 23)}<em>{label}</em></span>'
        for ic, label in tabs
    )
    acc = ""
    if accessory:
        acc = (
            '<div class="accessory glass">'
            + art("NM", 150, 30, 8)
            + '<span class="np"><b>Neon Meadow</b><i>The Salty Thunders</i></span>'
            '<span class="eq"><u style="height:6px"></u><u style="height:13px"></u>'
            '<u style="height:9px"></u><u style="height:4px"></u></span></div>'
        )
    return (
        f'<div class="bottom">{acc}<div class="bar-row"><div class="tabbar glass">{items}</div>'
        f'<span class="searchtab glass">{icon("search", 24)}</span></div></div>'
        '<div class="homeind"></div>'
    )


def toolbar(left, right):
    return f'<div class="edge"></div><div class="toolbar"><div class="tl">{left}</div><div class="tr">{right}</div></div>'


def range_pill(text="Last 4 weeks"):
    return f'<span class="pill glass"><i class="lamp"></i>{text}{icon("chev", 14, 2.4)}</span>'


def metric_pill(active="Plays"):
    a, b = ("on", "") if active == "Plays" else ("", "on")
    return f'<span class="seg glass"><b class="{a}">Plays</b><b class="{b}">Time</b></span>'


def circle(name):
    return f'<span class="circ glass">{icon(name, 20)}</span>'


def phone(theme, body, caption, apis):
    api_html = "".join(f"<li>{a}</li>" for a in apis)
    return (
        f'<figure><div class="phone {theme}"><div class="screen">{body}</div></div>'
        f"<figcaption><h3>{caption}</h3><ul>{api_html}</ul></figcaption></figure>"
    )


def dot_bars(values, rows=8):
    top = max(values)
    cols = []
    for v in values:
        lit = max(1, round(v / top * rows)) if v else 0
        dots = "".join(f'<i class="{"l" if r >= rows - lit else ""}"></i>' for r in range(rows))
        cols.append(f"<span>{dots}</span>")
    return f'<div class="dotbars">{"".join(cols)}</div>'


def summary_card(label, value, unit, delta):
    return (
        f'<div class="card stat"><span class="k">{label}</span>'
        f'<span class="v">{value}<small>{unit}</small></span>'
        f'<span class="d"><b>▲ {delta}</b> vs before</span></div>'
    )


def rank_row(n, initials, hue, title, sub, value, share, move, round_art=False):
    mv = {"new": '<em class="mv new">NEW</em>'}.get(move) or (
        f'<em class="mv up">▲ {move}</em>' if move > 0 else (f'<em class="mv dn">▼ {-move}</em>' if move < 0 else '<em class="mv">–</em>')
    )
    return (
        f'<div class="row"><span class="n">{n}</span>{art(initials, hue, 44, 22 if round_art else 9)}'
        f'<span class="t"><b>{title}</b><i>{sub}</i>'
        f'<u class="meter"><s style="width:{share}%"></s></u></span>'
        f'<span class="val"><b>{value}</b>{mv}</span></div>'
    )


# ---------- screens ----------

def screen_connect():
    heights = [2, 4, 3, 1]
    lamps = "".join(
        f'<span>{"".join(f"<i class={chr(34)}{ "l" if r >= 4 - h else "" }{chr(34)}></i>" for r in range(4))}</span>'
        for h in heights
    )
    body = (
        status_bar()
        + '<div class="connect">'
        f'<div class="mark">{lamps}</div>'
        '<div class="word">spotussi</div>'
        "<h1>Connect to your server</h1>"
        "<p>On your instance, open <b>Settings → Tokens</b> and choose <b>Pair iPhone</b>. Then scan the code.</p>"
        f'<span class="cta">{icon("qr", 20)} Scan pairing code</span>'
        '<span class="cta2 glass">Enter address and token</span>'
        '<p class="fine">Read-only access. The token is kept in the Keychain on this iPhone.</p>'
        "</div><div class=\"homeind\"></div>"
    )
    return phone(
        "dark",
        body,
        "1 · Connect",
        [
            "QR pairing: <code>spotussi://pair?url=…&amp;token=…</code>",
            "<code>DataScannerViewController</code> for the scan",
            "Token in Keychain, address in <code>@AppStorage</code>",
            "One prominent button, one glass button",
        ],
    )


def screen_overview():
    days = [58, 72, 64, 90, 110, 141, 96, 70, 66, 84, 77, 120, 176, 132, 61, 59, 88, 93, 71, 104, 150, 98, 64, 80, 76, 69, 112, 137]
    tops = [
        (1, "LN", 20, "Luca Nyvi", "36 plays · 2 h 5 min", "36", 100, 2),
        (2, "TR", 200, "Tide River", "32 plays · 1 h 48 min", "32", 88, 4),
        (3, "DN", 285, "DJ Neti", "30 plays · 1 h 25 min", "30", 83, -1),
        (4, "AL", 340, "Atlas Lighthouse", "26 plays · 1 h 31 min", "26", 72, "new"),
        (5, "MG", 95, "The Midnight Gardens", "24 plays · 1 h 12 min", "24", 66, 1),
    ]
    body = (
        '<div class="scroll">'
        '<div class="large"><h1>Overview</h1><span>Sep 21 – Oct 18, 2026</span></div>'
        '<div class="grid2">'
        + summary_card("Time listened", "136", " h", "37%")
        + summary_card("Plays", "2,348", "", "35%")
        + summary_card("Tracks", "1,931", "", "33%")
        + summary_card("Artists", "1,614", "", "31%")
        + "</div>"
        '<div class="card"><div class="ch"><b>Plays by day</b><span>Peak Wed, Oct 7 · 176</span></div>'
        + dot_bars(days)
        + '<div class="axis"><span>Sep 21</span><span>Oct 4</span><span>Oct 18</span></div></div>'
        '<div class="sh"><b>Top artists</b><span>All 1,614</span></div><div class="card list">'
        + "".join(rank_row(*t, round_art=True) for t in tops)
        + "</div></div>"
        + status_bar()
        + toolbar(range_pill(), metric_pill() + circle("gear"))
        + tabbar("Overview")
    )
    return phone(
        "dark",
        body,
        "2 · Overview",
        [
            "<code>TabView</code> with a separate <code>Tab(role: .search)</code>",
            "Now playing in <code>tabViewBottomAccessory</code>",
            "<code>tabBarMinimizeBehavior(.onScrollDown)</code>",
            "Range and Plays/Time live in the glass toolbar; cards stay solid",
        ],
    )


def screen_top():
    rows = [
        (1, "PO", 150, "The Paper Ocean", "The Scarlet Monsoons", "14", 100, 5),
        (2, "S", 175, "Season", "The Midnight Gardens", "13", 93, "new"),
        (3, "C", 130, "Cassette", "Garden Garden", "12", 86, 1),
        (4, "T", 350, "Turning", "DJ Neti", "10", 71, 3),
        (5, "CS", 225, "Calling Silver", "DJ Kamaqui", "9", 64, "new"),
        (6, "SS", 300, "Satellite in the Shadow", "DJ Selome", "9", 64, -2),
        (7, "CB", 30, "The Cobalt Shadow", "Velvet Compass", "8", 57, 0),
        (8, "VP", 320, "The Velvet Postcard", "Mara & the Mirrors", "8", 57, 6),
        (9, "H", 205, "Highways", "DJ Timoka", "7", 50, -4),
        (10, "FP", 260, "The Fragile Prism", "Sacred Ribbon", "7", 50, 1),
    ]
    body = (
        '<div class="scroll">'
        '<div class="large"><h1>Top</h1><span>Last 4 weeks · by plays</span></div>'
        '<div class="picker"><b class="on">Tracks</b><b>Artists</b><b>Albums</b><b>Genres</b></div>'
        '<div class="card list">'
        + "".join(rank_row(*r) for r in rows)
        + "</div></div>"
        + status_bar()
        + toolbar(range_pill(), metric_pill())
        + tabbar("Top", accessory=False)
    )
    return phone(
        "light",
        body,
        "3 · Top lists",
        [
            "Segmented <code>Picker</code> for the four kinds",
            "<code>List</code> with paged loading (no cap)",
            "Rank movement shown by shape and text, not colour alone",
            "Row taps push the entity page; context menu: Open in Spotify",
        ],
    )


def screen_artist():
    months = [3, 5, 2, 8, 12, 9, 4, 6, 14, 18, 11, 7, 9, 16, 22, 13, 8, 10, 19, 24, 17, 12, 28, 36]
    top = max(months)
    bars = "".join(f'<i style="height:{max(4, v / top * 62):.0f}px"></i>' for v in months)
    tracks = [
        (1, "PO", 150, "Paper Lanterns", "Low Tide Radio · 2021", "11", 100, 1),
        (2, "LT", 15, "Low Tide", "Low Tide Radio · 2021", "9", 82, 0),
        (3, "GH", 40, "Glasshouse", "Glasshouse · 2024", "7", 64, "new"),
    ]
    body = (
        '<div class="scroll nopad">'
        '<div class="hero"><div class="heroart"></div><div class="herotext">'
        '<span class="eyebrow">Artist</span><h1>Luca Nyvi</h1>'
        '<span class="chips"><i>french indie</i><i>nu indie</i><i>dream pop</i></span></div></div>'
        '<div class="pad">'
        '<div class="grid2">'
        '<div class="card stat"><span class="k">Rank, last 4 weeks</span><span class="v"><small>#</small>1</span><span class="d">of 1,614 artists</span></div>'
        '<div class="card stat"><span class="k">In range</span><span class="v">36</span><span class="d">2 h 5 min · 4 skipped</span></div>'
        '<div class="card stat"><span class="k">All time</span><span class="v">1,204</span><span class="d">71 h 12 min</span></div>'
        '<div class="card stat"><span class="k">First played</span><span class="v sm">Mar 8, 2012</span><span class="d">Last: today, 8:19 AM</span></div>'
        "</div>"
        f'<div class="card"><div class="ch"><b>Plays over time</b><span>by month</span></div><div class="vbars">{bars}</div></div>'
        '<div class="sh"><b>Most played tracks</b><span>All 48</span></div><div class="card list">'
        + "".join(rank_row(*t) for t in tracks)
        + "</div></div></div>"
        + status_bar()
        + toolbar(circle("back"), '<span class="group glass">' + icon("share", 20) + icon("more", 20) + "</span>")
        + tabbar("Top", accessory=False)
    )
    return phone(
        "dark",
        body,
        "4 · Artist page",
        [
            "Artwork runs under the bar: <code>backgroundExtensionEffect()</code>",
            "Toolbar buttons grouped in one glass capsule",
            "Zoom transition from the list row (<code>matchedTransitionSource</code>)",
            "Same layout for album, track and genre pages",
        ],
    )


def screen_history():
    plays = [
        ("8:19 AM", "SS", 300, "Satellite in the Shadow", "DJ Selome, The Golden Engines", "", True),
        ("8:18 AM", "CB", 30, "The Cobalt Shadow", "Velvet Compass", "skipped at 0:22", False),
        ("8:15 AM", "VP", 320, "The Velvet Postcard", "Mara & the Mirrors", "", True),
        ("8:15 AM", "H", 205, "Highways", "DJ Timoka", "skipped at 0:18", False),
        ("8:11 AM", "FP", 260, "The Fragile Prism", "Sacred Ribbon", "", True),
    ]
    late = [
        ("11:44 PM", "ML", 150, "Meadow in the Lantern", "Namave, DJ Monra", "", True),
        ("11:41 PM", "WR", 215, "Wandering River", "Lena Bena", "", True),
        ("11:41 PM", "R", 120, "Running", "DJ Elnana", "skipped at 0:21", False),
        ("11:39 PM", "EC", 345, "Empty Signal", "Theo & the Cathedrals", "", True),
    ]

    def prow(t, ini, hue, title, sub, note, counted):
        n = f'<em class="note">{note}</em>' if note else ""
        lamp = '<i class="lamp"></i>' if counted else '<i class="lamp off"></i>'
        return (
            f'<div class="row">{art(ini, hue, 42, 8)}<span class="t"><b>{title}</b><i>{sub}</i></span>'
            f'<span class="val"><b class="tm">{t}</b>{n or lamp}</span></div>'
        )

    body = (
        '<div class="scroll">'
        '<div class="large"><h1>History</h1><span>2,348 plays in the last 4 weeks</span></div>'
        '<div class="picker two"><b class="on">Plays</b><b>Sessions</b></div>'
        '<div class="chiprow"><i class="on">All</i><i>Skipped</i><i>Shuffle</i><i>iPhone</i><i>Living Room</i></div>'
        '<div class="dh">Today</div><div class="card list">'
        + "".join(prow(*p) for p in plays)
        + '</div><div class="dh">Yesterday</div><div class="card list">'
        + "".join(prow(*p) for p in late)
        + "</div></div>"
        + status_bar()
        + toolbar(range_pill(), circle("filter"))
        + tabbar("History", accessory=False)
    )
    return phone(
        "light",
        body,
        "5 · History and sessions",
        [
            "Day-sectioned <code>List</code>, cursor paging from <code>/history</code>",
            "Filters in a toolbar <code>Menu</code>; active ones as chips",
            "Sessions share the tab (segmented), so the bar stays at five",
            "<code>.refreshable</code>; a lit lamp marks a counted play",
        ],
    )


def screen_stats():
    hours = [4, 2, 1, 1, 1, 2, 6, 14, 22, 30, 38, 33, 27, 24, 26, 29, 31, 28, 25, 30, 26, 18, 12, 7]
    top = max(hours)
    cx = cy = 130
    dots = []
    for h, v in enumerate(hours):
        ang = math.radians(h * 15 - 90)
        lit = max(1, round(v / top * 6))
        for k in range(6):
            r = 52 + k * 11
            x, y = cx + r * math.cos(ang), cy + r * math.sin(ang)
            cls = "l" if k < lit else ""
            dots.append(f'<circle class="{cls}" cx="{x:.1f}" cy="{y:.1f}" r="3.4"/>')
    labels = (
        '<text x="130" y="12">12 AM</text><text x="252" y="134">6 AM</text>'
        '<text x="130" y="256">12 PM</text><text x="10" y="134">6 PM</text>'
    )
    clock = (
        f'<svg class="clock" viewBox="0 0 260 262" width="260" height="262">{"".join(dots)}{labels}'
        '<text class="big" x="130" y="128">10 AM</text><text class="sub" x="130" y="146">busiest hour</text></svg>'
    )
    genres = [("brazilian indie", 88, 0), ("french indie", 71, 1), ("experimental indie", 64, 2), ("nu indie", 58, 3)]
    cols = []
    for w in range(12):
        parts = [random.randint(8, 30) for _ in range(4)]
        cols.append("<span>" + "".join(f'<i class="g{i}" style="height:{p}px"></i>' for i, p in enumerate(parts)) + "</span>")
    legend = "".join(f'<span><i class="g{i}"></i>{name}</span>' for name, _, i in genres)
    body = (
        '<div class="scroll">'
        '<div class="large"><h1>Stats</h1><span>Last 4 weeks</span></div>'
        f'<div class="card"><div class="ch"><b>Listening clock</b><span>quietest 3 AM</span></div><div class="center">{clock}</div></div>'
        f'<div class="card"><div class="ch"><b>Genres over time</b><span>by week</span></div><div class="stack">{"".join(cols)}</div>'
        f'<div class="legend">{legend}</div></div>'
        "</div>"
        + status_bar()
        + toolbar(range_pill(), metric_pill("Time") + circle("gear"))
        + tabbar("Stats")
    )
    return phone(
        "dark",
        body,
        "6 · Stats",
        [
            "Listening clock drawn in <code>Canvas</code> (the brand's lamp grid)",
            "Time series in Swift Charts with <code>chartXSelection</code> scrubbing",
            "Charts carry <code>accessibilityChartDescriptor</code> for VoiceOver",
            "Five chart colours from the web palette, checked for colour vision",
        ],
    )


def screen_library():
    def lamps(n):
        return '<u class="rot">' + "".join(f'<s class="{"l" if i < n else ""}"></s>' for i in range(5)) + "</u>"

    songs = [
        ("GH", 40, "Glasshouse", "Luca Nyvi", "Saved Oct 14", "7", 5),
        ("NM", 150, "Neon Meadow", "The Salty Thunders", "Saved Oct 11", "3", 3),
        ("QS", 215, "Quiet Signal", "Lena Bena", "Saved Oct 9", "0", 0),
        ("PL", 20, "Paper Lanterns", "Luca Nyvi", "Saved Oct 2", "11", 5),
        ("HR", 300, "Harbour Lights", "Atlas Lighthouse", "Saved Sep 28", "1", 1),
        ("SM", 95, "Small Hours", "The Midnight Gardens", "Saved Sep 21", "0", 0),
    ]
    rows = "".join(
        f'<div class="row">{art(i, h, 44, 9)}<span class="t"><b>{t}</b><i>{a} · {d}</i></span>'
        f'<span class="val"><b>{p}</b>{lamps(n)}</span></div>'
        for i, h, t, a, d, p, n in songs
    )
    mix = [("g0", 18), ("m1", 14), ("m2", 12), ("m3", 9), ("m4", 21), ("m5", 26)]
    bar = "".join(f'<i class="{c}" style="flex:{w}"></i>' for c, w in mix)
    body = (
        '<div class="scroll">'
        '<div class="large"><h1>Library</h1><span>All time · synced 2 hr ago</span></div>'
        '<div class="grid2">'
        '<div class="card stat"><span class="k">Saved, never played</span><span class="v">212</span><span class="d">of 1,284 songs</span></div>'
        '<div class="card stat"><span class="k">To heavy rotation</span><span class="v">9<small> days</small></span><span class="d">median of 388 songs</span></div>'
        "</div>"
        f'<div class="card"><div class="ch"><b>When saved songs reached rotation</b></div><div class="mix">{bar}</div>'
        '<div class="axis"><span>Before saving</span><span>Within a month</span><span>Not yet</span></div></div>'
        '<div class="picker"><b class="on">Songs 1,284</b><b>Albums 96</b><b>Playlists 41</b></div>'
        f'<div class="card list">{rows}</div></div>'
        + status_bar()
        + toolbar('<span class="pill glass">Recently saved' + icon("chev", 14, 2.4) + "</span>", circle("gear"))
        + tabbar("Library", accessory=False)
    )
    return phone(
        "dark",
        body,
        "7 · Library",
        [
            "No range control: saves carry their own dates (D36)",
            "Sort in a toolbar <code>Menu</code>; Songs, Albums, Playlists in a <code>Picker</code>",
            "Five lamps per song: plays towards heavy rotation",
            "Read-only; sync and the Spotify permission stay on the web",
        ],
    )


CSS = r"""
*{box-sizing:border-box;margin:0;padding:0}
:root{color-scheme:light dark;--page:#e9eaed;--ptext:#16171b;--pmuted:#5d616c;--pline:#d3d5da;--pcard:#f6f6f8}
@media (prefers-color-scheme:dark){:root{--page:#0c0d10;--ptext:#eceef1;--pmuted:#9a9eaa;--pline:#262930;--pcard:#15171b}}
body{background:var(--page);color:var(--ptext);font:15px/1.45 -apple-system,"SF Pro Text",ui-sans-serif,system-ui,sans-serif;padding:28px 20px 60px;-webkit-font-smoothing:antialiased}
header{max-width:1180px;margin:0 auto 26px}
header h1{font-size:28px;letter-spacing:-.02em;font-weight:700}
header p{color:var(--pmuted);max-width:70ch;margin-top:6px}
.notes{display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}
.notes span{border:1px solid var(--pline);background:var(--pcard);border-radius:999px;padding:5px 12px;font-size:13px;color:var(--pmuted)}
.notes b{color:var(--ptext);font-weight:600}
main{max-width:1180px;margin:0 auto;display:grid;grid-template-columns:repeat(auto-fill,minmax(330px,1fr));gap:34px 22px}
figure{display:flex;flex-direction:column;align-items:center}
figcaption{width:320px;margin-top:14px}
figcaption h3{font-size:15px;font-weight:650}
figcaption ul{margin:6px 0 0 16px;color:var(--pmuted);font-size:12.5px;line-height:1.5}
code{font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--ptext);opacity:.85}

.phone{width:390px;height:844px;zoom:.82;border-radius:58px;padding:9px;background:#0a0a0c;box-shadow:0 0 0 1.5px #3b3d44,0 30px 60px -20px rgba(0,0,0,.5);flex:none}
.screen{position:relative;width:100%;height:100%;border-radius:50px;overflow:hidden;background:var(--bg);color:var(--fg);font-size:15px}
.dark{--bg:#121317;--card:#1a1c21;--fg:#eceef1;--muted:#a4a8b3;--faint:#7d8190;--line:#272a31;--grille:#3a3d46;--signal:#1db954;--ink:#1ed760;
 --glass:rgba(118,122,134,.30);--edge:rgba(255,255,255,.24);--hi:rgba(255,255,255,.22);--sel:rgba(10,11,14,.46);--gshadow:0 8px 24px rgba(0,0,0,.35);
 --c0:#1db954;--c1:#5683d6;--c2:#d9577f;--c3:#9a72dc}
.light{--bg:#edeef0;--card:#fbfbfc;--fg:#16171b;--muted:#5d616c;--faint:#7c808a;--line:#e1e3e7;--grille:#cfd1d4;--signal:#169c45;--ink:#107a35;
 --glass:rgba(255,255,255,.58);--edge:rgba(255,255,255,.95);--hi:rgba(255,255,255,.9);--sel:rgba(22,23,27,.09);--gshadow:0 0 0 .5px rgba(0,0,0,.10),0 8px 22px rgba(20,22,30,.13);
 --c0:#169c45;--c1:#3a6bc4;--c2:#c2406a;--c3:#8a5ccf}
.glass{background:var(--glass);-webkit-backdrop-filter:blur(16px) saturate(190%);backdrop-filter:blur(16px) saturate(190%);border:1px solid var(--edge);box-shadow:inset 0 1px 0 var(--hi),var(--gshadow)}

.status{position:absolute;inset:0 0 auto 0;height:54px;display:flex;align-items:center;justify-content:space-between;padding:6px 34px 0;z-index:6;font-weight:600;font-size:16px}
.island{position:absolute;left:50%;top:11px;width:118px;height:34px;margin-left:-59px;border-radius:20px;background:#000}
.sys{display:flex;gap:7px;align-items:center}
.bars{display:flex;gap:2px;align-items:flex-end;height:11px}.bars b{width:3px;background:currentColor;border-radius:1px}
.bars b:nth-child(1){height:4px}.bars b:nth-child(2){height:6px}.bars b:nth-child(3){height:8.5px}.bars b:nth-child(4){height:11px}
.batt{width:25px;height:12px;border:1.2px solid currentColor;border-radius:4px;padding:1.4px;opacity:.9}.batt b{display:block;width:78%;height:100%;background:currentColor;border-radius:1.5px}
.homeind{position:absolute;left:50%;bottom:8px;width:136px;height:5px;margin-left:-68px;border-radius:3px;background:var(--fg);opacity:.85;z-index:7}

.scroll{position:absolute;inset:0;padding:112px 16px 0;overflow:hidden}
.scroll.nopad{padding:0}.pad{padding:14px 16px 0}
.edge{position:absolute;inset:0 0 auto 0;height:118px;z-index:4;pointer-events:none;background:linear-gradient(var(--bg) 38%,color-mix(in srgb,var(--bg) 70%,transparent) 68%,transparent)}
.scroll.nopad ~ .edge{display:none}
.toolbar{position:absolute;inset:58px 16px auto 16px;display:flex;justify-content:space-between;align-items:center;z-index:5}
.tl,.tr{display:flex;gap:8px;align-items:center}
.pill{display:inline-flex;align-items:center;gap:6px;height:44px;padding:0 13px;white-space:nowrap;border-radius:22px;font-weight:600;font-size:15px}
.lamp{width:8px;height:8px;border-radius:50%;background:var(--signal);box-shadow:0 0 6px color-mix(in srgb,var(--signal) 70%,transparent);flex:none}
.lamp.off{background:var(--grille);box-shadow:none}
.seg{display:inline-flex;height:44px;padding:4px;border-radius:22px;font-size:14px;font-weight:600}
.seg b{display:flex;align-items:center;padding:0 11px;border-radius:18px;font-weight:600;color:var(--muted)}
.seg b.on{background:var(--sel);color:var(--fg)}
.circ{display:inline-flex;width:44px;height:44px;border-radius:50%;align-items:center;justify-content:center}
.group{display:inline-flex;height:44px;border-radius:22px;align-items:center;gap:18px;padding:0 15px}

.large{margin:2px 2px 14px}.large h1{font-size:34px;font-weight:750;letter-spacing:-.025em;font-stretch:112%;line-height:1.1}
.large span{color:var(--muted);font-size:14px}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:10px}
.card{background:var(--card);border-radius:22px;padding:14px 16px;margin-bottom:10px;border:.5px solid var(--line)}
.stat{display:flex;flex-direction:column;gap:2px;margin:0;padding:13px 15px}
.k{font-size:12.5px;color:var(--muted);font-weight:500}
.v{font-size:32px;font-weight:720;letter-spacing:-.03em;font-stretch:118%;line-height:1.1;font-variant-numeric:tabular-nums}
.v small{font-size:17px;font-weight:600;color:var(--muted);margin:0 1px}.v.sm{font-size:19px;letter-spacing:-.01em;padding:7px 0 6px}
.d{font-size:12px;color:var(--faint)}.d b{color:var(--ink);font-weight:600}
.ch{display:flex;justify-content:space-between;align-items:baseline;margin-bottom:12px}.ch b{font-size:15px;font-weight:650}.ch span{font-size:12px;color:var(--faint)}
.dotbars{display:flex;justify-content:space-between}.dotbars span{display:flex;flex-direction:column;gap:3.5px}
.dotbars i{width:7px;height:7px;border-radius:50%;background:var(--grille)}.dotbars i.l{background:var(--signal)}
.axis{display:flex;justify-content:space-between;font-size:11px;color:var(--faint);margin-top:8px}
.sh{display:flex;justify-content:space-between;align-items:baseline;margin:16px 4px 8px}.sh b{font-size:20px;font-weight:700;letter-spacing:-.01em}.sh span{font-size:13px;color:var(--ink);font-weight:500}
.list{padding:2px 14px}
.row{display:flex;align-items:center;gap:11px;padding:9px 0;border-bottom:.5px solid var(--line);min-height:62px}
.row:last-child{border:0}
.n{width:17px;text-align:center;font-size:13px;color:var(--faint);font-weight:600;font-variant-numeric:tabular-nums;flex:none}
.art{display:inline-flex;align-items:center;justify-content:center;color:rgba(255,255,255,.92);font-weight:700;flex:none;letter-spacing:.02em}
.t{flex:1;min-width:0;display:flex;flex-direction:column;gap:1px}
.t b{font-size:15px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.t i{font-style:normal;font-size:12.5px;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.meter{display:block;height:3px;border-radius:2px;background:var(--grille);margin-top:5px;text-decoration:none}.meter s{display:block;height:100%;border-radius:2px;background:var(--signal)}
.val{display:flex;flex-direction:column;align-items:flex-end;gap:2px;flex:none}
.val b{font-size:16px;font-weight:700;font-variant-numeric:tabular-nums}.val b.tm{font-size:12.5px;font-weight:500;color:var(--muted)}
.mv{font-style:normal;font-size:11px;color:var(--faint);font-weight:600}.mv.up{color:var(--muted)}.mv.new{color:var(--ink);letter-spacing:.04em}
.note{font-style:normal;font-size:11px;color:var(--faint)}

.picker{display:flex;background:color-mix(in srgb,var(--fg) 8%,transparent);border-radius:20px;padding:3px;margin-bottom:12px}
.picker b{flex:1;text-align:center;font-size:13.5px;font-weight:600;padding:7px 0;border-radius:17px;color:var(--muted)}
.picker b.on{background:var(--card);color:var(--fg);box-shadow:0 1px 4px rgba(0,0,0,.14)}
.chiprow{display:flex;gap:7px;margin-bottom:6px;overflow:hidden;white-space:nowrap}
.chiprow i{font-style:normal;font-size:13px;font-weight:550;padding:6px 12px;border-radius:16px;border:1px solid var(--line);color:var(--muted);background:var(--card)}
.chiprow i.on{background:var(--fg);color:var(--bg);border-color:var(--fg)}
.dh{font-size:13px;font-weight:650;color:var(--muted);margin:12px 6px 7px;text-transform:uppercase;letter-spacing:.05em}

.bottom{position:absolute;left:14px;right:14px;bottom:24px;z-index:6;display:flex;flex-direction:column;gap:9px}
.bottom::before{content:"";position:absolute;inset:-46px -14px -24px;z-index:-1;background:linear-gradient(transparent,color-mix(in srgb,var(--bg) 60%,transparent) 60%)}
.accessory{height:48px;border-radius:24px;display:flex;align-items:center;gap:10px;padding:0 16px 0 9px}
.np{flex:1;min-width:0;display:flex;flex-direction:column;line-height:1.2}.np b{font-size:14px;font-weight:600}.np i{font-style:normal;font-size:12px;color:var(--muted)}
.eq{display:flex;gap:2.5px;align-items:flex-end;height:14px}.eq u{width:3px;border-radius:1.5px;background:var(--signal)}
.bar-row{display:flex;gap:10px;align-items:center}
.tabbar{flex:1;height:62px;border-radius:31px;display:flex;padding:4px}
.tab{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:1px;border-radius:27px;color:var(--fg)}
.tab em{font-style:normal;font-size:10.5px;font-weight:600}
.tab.on{background:var(--sel);color:var(--ink)}
.searchtab{width:62px;height:62px;border-radius:50%;display:flex;align-items:center;justify-content:center;flex:none}

.hero{position:relative;height:330px}
.heroart{position:absolute;inset:0;background:radial-gradient(120% 90% at 20% 15%,hsl(24 85% 58%),transparent 60%),radial-gradient(90% 80% at 85% 30%,hsl(330 70% 48%),transparent 62%),linear-gradient(160deg,hsl(40 80% 46%),hsl(280 55% 26%))}
.heroart::after{content:"";position:absolute;inset:0;background:linear-gradient(transparent 45%,var(--bg))}
.herotext{position:absolute;left:18px;right:18px;bottom:6px}
.eyebrow{font-size:12.5px;font-weight:650;text-transform:uppercase;letter-spacing:.08em;opacity:.85}
.herotext h1{font-size:40px;font-weight:780;letter-spacing:-.03em;font-stretch:118%;line-height:1.05;margin:2px 0 9px}
.chips{display:flex;gap:6px}.chips i{font-style:normal;font-size:12.5px;padding:4px 10px;border-radius:13px;background:rgba(255,255,255,.14);border:.5px solid rgba(255,255,255,.22)}
.vbars{display:flex;align-items:flex-end;gap:5px;height:64px}.vbars i{flex:1;border-radius:3px 3px 1px 1px;background:var(--signal);opacity:.92}

.center{display:flex;justify-content:center}
.clock circle{fill:var(--grille)}.clock circle.l{fill:var(--signal)}
.clock text{fill:var(--faint);font-size:10px;text-anchor:middle;font-weight:600}
.clock text.big{fill:var(--fg);font-size:24px;font-weight:750;letter-spacing:-.02em}.clock text.sub{font-size:11px;font-weight:500}
.stack{display:flex;align-items:flex-end;gap:7px;height:92px}.stack span{flex:1;display:flex;flex-direction:column-reverse;gap:2px}
.stack i{display:block;border-radius:2.5px}.g0{background:var(--c0)}.g1{background:var(--c1)}.g2{background:var(--c2)}.g3{background:var(--c3)}
.legend{display:flex;flex-wrap:wrap;gap:6px 12px;margin-top:11px;font-size:12px;color:var(--muted)}
.legend span{display:flex;align-items:center;gap:5px}.legend i{width:8px;height:8px;border-radius:2px}

.rot{display:flex;gap:3px;text-decoration:none}.rot s{width:6px;height:6px;border-radius:50%;background:var(--grille)}.rot s.l{background:var(--signal)}
.mix{display:flex;gap:2px;height:14px}.mix i{border-radius:3px;background:var(--grille)}.mix .g0{background:var(--signal)}
.mix .m1{background:color-mix(in srgb,var(--signal) 78%,var(--card))}.mix .m2{background:color-mix(in srgb,var(--signal) 56%,var(--card))}
.mix .m3{background:color-mix(in srgb,var(--signal) 36%,var(--card))}.mix .m4{background:color-mix(in srgb,var(--fg) 40%,var(--card))}.mix .m5{background:var(--grille)}
.tab em{white-space:nowrap}
.connect{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:0 30px 40px;text-align:center;
 background:radial-gradient(70% 40% at 50% 28%,rgba(29,185,84,.16),transparent 70%)}
.mark{display:flex;gap:9px;padding:22px;border-radius:30px;background:#1a1c21;border:.5px solid #2c2f37;margin-bottom:16px}
.mark span{display:flex;flex-direction:column;gap:9px}.mark i{width:13px;height:13px;border-radius:50%;background:#3a3d46}.mark i.l{background:#1db954;box-shadow:0 0 10px rgba(29,185,84,.55)}
.word{font-size:24px;font-weight:760;letter-spacing:-.035em;font-stretch:125%;margin-bottom:54px}
.connect h1{font-size:27px;font-weight:740;letter-spacing:-.02em;margin-bottom:10px}
.connect p{color:var(--muted);font-size:15.5px;line-height:1.45}.connect p b{color:var(--fg);font-weight:600}
.cta{display:flex;align-items:center;justify-content:center;gap:9px;width:100%;height:54px;border-radius:27px;background:#1db954;color:#06210f;font-weight:700;font-size:17px;margin-top:30px;
 box-shadow:inset 0 1px 0 rgba(255,255,255,.45),0 10px 26px rgba(29,185,84,.28)}
.cta2{display:flex;align-items:center;justify-content:center;width:100%;height:54px;border-radius:27px;font-weight:600;font-size:17px;margin-top:11px}
.fine{font-size:12.5px!important;margin-top:22px;color:var(--faint)!important}
"""


RENDER_CSS = r"""
:root{--page:transparent;--ptext:var(--foreground);--pmuted:var(--muted-foreground);--pline:var(--border);--pcard:var(--card)}
body{background:none;padding:0}
header{margin-bottom:18px}
main{grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:30px 12px}
.phone{zoom:.8}
figcaption{width:300px}
"""


def build(render=False):
    screens = [screen_connect(), screen_overview(), screen_top(), screen_artist(), screen_history(), screen_stats(), screen_library()]
    notes = [
        "<b>Glass</b> only on the navigation layer: tab bar, toolbar, now-playing accessory",
        "<b>Content</b> stays on solid graphite and aluminium panels",
        "<b>One colour:</b> green marks something lit",
        "<b>Sample data</b> from the mock server, not real listening",
    ]
    return (
        '<!doctype html><html lang="en"><head><meta charset="utf-8">'
        '<meta name="viewport" content="width=device-width,initial-scale=1">'
        "<title>spotussi iPhone mockups</title>"
        f"<style>{CSS}{RENDER_CSS if render else ''}</style></head><body>"
        "<header>" + ("" if render else "<h1>spotussi for iPhone: MVP mockups</h1>") +
        "<p>Seven screens of the read-only companion app, drawn for iOS 27 with Liquid Glass. "
        "These are HTML sketches of layout and hierarchy; the real glass, type and controls come from SwiftUI's system components.</p>"
        f'<div class="notes">{"".join(f"<span>{n}</span>" for n in notes)}</div></header>'
        f'<main>{"".join(screens)}</main></body></html>'
    )


if __name__ == "__main__":
    out = sys.argv[1]
    with open(out, "w") as f:
        f.write(build(render="--render" in sys.argv))
    print("wrote", out)
