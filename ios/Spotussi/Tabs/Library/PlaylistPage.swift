import SpotussiAPI
import SpotussiKit
import SwiftUI

/// One playlist, as the web's playlist page: whose it is, its size, how
/// much of it was ever played and the plays started from it, then its
/// songs in playlist order, each opening its track. Spotify shares the
/// songs of your own and collaborative playlists only, so a followed one
/// shows its plays alone.
struct PlaylistPage: View {
    let link: PlaylistLink

    @Environment(Session.self) private var session
    @Environment(\.openURL) private var openURL
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var model: PlaylistModel
    @Namespace private var zoom

    init(link: PlaylistLink) {
        self.link = link
        _model = State(initialValue: PlaylistModel(id: link.id))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if model.isNotFound {
                    Text("Not in your library.")
                        .foregroundStyle(.secondary)
                        .panel()
                } else if model.isFailed, model.playlist == nil {
                    SectionFailure().panel()
                } else {
                    header
                    figures
                        .redacted(reason: model.playlist == nil ? .placeholder : [])
                    songs
                }
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 32)
        }
        .background(.ground)
        .navigationTitle(model.playlist?.name ?? link.name)
        .navigationBarTitleDisplayMode(.inline)
        // The name leads the page; in the bar it would only repeat it.
        .toolbar(removing: .title)
        .toolbar {
            if let url = model.playlist.flatMap({ SpotifyLink.url(for: $0.spotifyURI) }) {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        openURL(url)
                    } label: {
                        Label("Open in Spotify", systemImage: "arrow.up.forward.app")
                    }
                }
            }
        }
        .task { await model.load(client: client) }
        .refreshable { [model, client] in
            await model.load(client: client, force: true)
        }
    }

    // MARK: - Head

    private var header: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 12))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 16))
        return layout {
            ArtworkImage(url: model.playlist?.imageURL, side: 88, symbol: "music.note.list")
            VStack(alignment: .leading, spacing: 4) {
                if let playlist = model.playlist {
                    Text(PlaylistRow.owner(playlist))
                        .font(.caption.weight(.semibold).width(.expanded))
                        .textCase(.uppercase)
                        .foregroundStyle(.secondary)
                }
                Text(model.playlist?.name ?? link.name)
                    .font(.system(.title, weight: .bold).width(.expanded))
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isHeader)
        }
    }

    // MARK: - Figures

    private var figures: some View {
        let playlist = model.playlist
        let columns = Array(
            repeating: GridItem(.flexible(), spacing: 12, alignment: .top),
            count: dynamicTypeSize.isAccessibilitySize ? 1 : 2)
        return LazyVGrid(columns: columns, spacing: 12) {
            Readout(label: "Songs", value: playlist?.itemsTotal.map { Figures.count($0) } ?? "–")
            if let playThrough = playlist?.playThrough {
                Readout(
                    label: "Ever played", value: Figures.count(playThrough.played),
                    detail: String(localized: "of \(playThrough.total)"))
            } else {
                Readout(label: "Ever played", value: "–", spokenValue: String(localized: "Not known"))
            }
            Readout(
                label: "Plays from it", value: Figures.count(playlist?.playsHere ?? 0),
                detail: Figures.duration(milliseconds: playlist?.millisecondsHere ?? 0))
            Readout(
                label: "Last played", value: playlist?.lastPlayedHereAt.map { Figures.date($0, now: .now) } ?? "–",
                spokenValue: playlist?.lastPlayedHereAt == nil ? String(localized: "Never") : nil)
        }
    }

    // MARK: - Songs

    @ViewBuilder private var songs: some View {
        if let playlist = model.playlist, !playlist.itemsAvailable {
            Text("Songs not shared by Spotify.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .panel()
        } else {
            PagedPanel(model: model.songs, query: link.id, empty: "No songs.") {
                SongRow(song: .placeholder)
            } row: { song in
                let origin = ZoomOrigin(id: "playlist:\(song.id)", namespace: zoom)
                RouteLink(
                    route: .entity(song.track, zoom: origin), accessibilityLabel: SongRow.spokenLabel(song),
                    accessibilityValue: SongRow.spokenValue(song, quiet: false, now: .now),
                    spotifyURI: song.spotifyURI
                ) {
                    SongRow(song: song, zoom: origin)
                }
            }
        }
    }

    private var client: Client {
        session.client ?? .demo
    }
}

#Preview {
    PreviewHost(.demo) {
        NavigationStack {
            PlaylistPage(link: PlaylistLink(id: "Yj2bx8jZGsVOSKxL4h1bm9", name: "Running Winter"))
                .routeDestinations()
        }
    }
}

#Preview("Largest text") {
    PreviewHost(.demo) {
        NavigationStack {
            PlaylistPage(link: PlaylistLink(id: "Yj2bx8jZGsVOSKxL4h1bm9", name: "Running Winter"))
                .routeDestinations()
        }
    }
    .dynamicTypeSize(.accessibility5)
}
