#!/usr/bin/env python3
"""Records the app's sample data from `spotussi serve --mock`.

One JSON file per API operation, named by its operationId, written to
ios/SpotussiKit/Sources/SpotussiKit/Fixtures. The app's demo mode, its
previews and its tests read them (DemoTransport.swift).

    ios/scripts/record-fixtures.py

Builds the server from this checkout and runs the small dataset with a fixed
seed in UTC. The dataset is anchored to today, so every run changes dates.
"""

import json
import os
import pathlib
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / "ios/SpotussiKit/Sources/SpotussiKit/Fixtures"

# Lists are kept short: enough rows to fill a phone screen and scroll.
LISTS = "limit=25"


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        binary = os.path.join(tmp, "spotussi")
        subprocess.run(["go", "build", "-o", binary, "./cmd/spotussi"], cwd=ROOT, check=True)
        port = free_port()
        server = subprocess.Popen(
            [binary, "serve", "--mock", "--mock-size", "small", "--mock-seed", "1", "--mock-tz", "UTC"],
            env={**os.environ, "HOST": "127.0.0.1", "PORT": str(port)},
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        try:
            record(f"http://127.0.0.1:{port}/api/v1")
        finally:
            server.terminate()
            server.wait()
    return 0


def record(base: str) -> None:
    def get(path: str):
        with urllib.request.urlopen(base + path, timeout=30) as r:
            return json.load(r)

    for _ in range(100):
        try:
            get("/health")
            break
        except (urllib.error.URLError, ConnectionError):
            time.sleep(0.1)
    else:
        sys.exit("the mock server didn't start")

    OUT.mkdir(parents=True, exist_ok=True)
    for old in OUT.glob("*.json"):
        old.unlink()

    def save(operation: str, path: str):
        body = get(path)
        text = json.dumps(body, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
        (OUT / f"{operation}.json").write_text(text + "\n", encoding="utf-8")
        return body

    save("getHealth", "/health")
    save("getSystemStatus", "/system/status")
    save("getSpotifyConnection", "/spotify/connection")
    save("getSettings", "/settings")
    save("getNowPlaying", "/now-playing")

    save("getOverviewSummary", "/overview/summary")
    save("getTimeline", "/timeline?compare=true")
    save("getClock", "/clock")
    tracks = save("getTopTracks", f"/top/tracks?{LISTS}")
    artists = save("getTopArtists", f"/top/artists?{LISTS}")
    albums = save("getTopAlbums", f"/top/albums?{LISTS}")
    genres = save("getTopGenres", f"/top/genres?{LISTS}")

    save("getHourlyTop", "/stats/hourly-top")
    save("getArtistsOverTime", "/stats/artists-over-time")
    save("getGenresOverTime", "/stats/genres-over-time")
    save("getReleaseYears", "/stats/release-years")
    save("getArtistsPerTrack", "/stats/artists-per-track")

    save("getArtist", "/artists/" + artists["items"][0]["artist"]["id"])
    save("getAlbum", "/albums/" + albums["items"][0]["album"]["id"])
    save("getTrack", "/tracks/" + tracks["items"][0]["track"]["id"])
    save("getGenre", "/genres/" + genres["items"][0]["genre"]["id"])

    save("getHistory", f"/history?{LISTS}")
    save("getDevices", "/devices")
    save("getContexts", f"/contexts?{LISTS}")
    sessions = save("getSessions", f"/sessions?{LISTS}")
    save("getSession", "/sessions/" + sessions["items"][0]["id"])

    save("getLibrarySummary", "/library/summary")
    save("getLibraryTracks", f"/library/tracks?{LISTS}")
    save("getLibraryAlbums", f"/library/albums?{LISTS}")
    playlists = save("getLibraryPlaylists", f"/library/playlists?{LISTS}")
    save("getLibraryPlaylist", "/library/playlists/" + playlists["items"][0]["playlist"]["id"])
    save("getLibraryTimeline", "/library/timeline")

    save("search", "/search?q=" + artists["items"][0]["artist"]["name"][:3])

    files = sorted(OUT.glob("*.json"))
    total = sum(f.stat().st_size for f in files)
    print(f"{len(files)} fixtures, {total / 1024:.0f} KB, in {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    sys.exit(main())
