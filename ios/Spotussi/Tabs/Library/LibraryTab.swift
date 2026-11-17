import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The songs, albums and playlists saved on Spotify, held against the
/// listening (D36), as the web's Library: the readouts and when saves
/// reached heavy rotation, songs saved over the years, then each list with
/// its own sort and filter. No range or measure: saves carry their own
/// dates. When there is nothing to show, one line says why.
struct LibraryTab: View {
    @Environment(Session.self) private var session
    @Environment(InstanceStatus.self) private var status
    @State private var model = LibraryModel()
    /// The list switch has scrolled up past the top.
    @State private var listsAreAbove = false
    @State private var scrollTarget: ScrollTarget?
    @Namespace private var zoom

    private static let listsID = "library.lists"

    /// How often an empty library asks again while its first sync runs.
    private static let syncPoll = Duration.seconds(10)

    var body: some View {
        let condition = condition
        TabPage(
            title: "Library", subtitle: subtitle, controls: .settingsOnly, scrollTarget: scrollTarget,
            refresh: { [model, client] in
                await model.load(client: client)
                await model.loadPart(client: client, force: true)
            }
        ) {
            content(condition)
        }
        .task { await model.load(client: client) }
        .task(id: model.shown) { await model.loadPart(client: client) }
        // Another list, sort or filter starts at its top, under the switch,
        // rather than wherever the old list was scrolled to.
        .onChange(of: model.shown) {
            guard listsAreAbove else { return }
            scrollTarget = ScrollTarget(id: Self.listsID, request: (scrollTarget?.request ?? 0) + 1)
        }
        .task(id: condition?.empty == .firstSync) {
            guard condition?.empty == .firstSync else { return }
            while !Task.isCancelled {
                try? await Task.sleep(for: Self.syncPoll)
                guard !Task.isCancelled else { return }
                await status.refresh()
                await model.load(client: client)
            }
        }
    }

    @ViewBuilder private func content(_ condition: LibraryCondition?) -> some View {
        if model.summary == nil, model.summaryFailed {
            SectionFailure().panel()
        } else if let condition, let empty = condition.empty {
            LibraryStatePanel(empty: empty, syncing: condition.syncing)
        } else {
            if condition?.needsScope == true {
                LibraryScopeNotice()
            }
            LibraryFigures(summary: model.summary)
                .placeholder(model.summary == nil)
            RotationMixPanel(
                mix: model.summary.map { RotationMix($0.rotationMix) },
                rotationPlays: model.summary?.rotationPlays ?? SavedSong.rotationPlays)
            SavesPanel(timeline: model.timeline, isFailed: model.timelineFailed && model.timeline == nil)
            LibraryPartPicker(selection: $model.part)
                .padding(.top, 8)
                .id(Self.listsID)
                .onGeometryChange(for: Bool.self) { $0.frame(in: .scrollView).minY < 0 } action: {
                    listsAreAbove = $0
                }
            LibraryListBar(model: model)
            list
        }
    }

    @ViewBuilder private var list: some View {
        switch model.part {
        case .songs:
            // Until the summary says which filter leads, the panel shows
            // placeholders for the default.
            let query = model.songsQuery ?? SavedSong.Query(filter: .neverPlayed, sort: model.songSort)
            PagedPanel(
                model: model.songs, query: query,
                empty: query.filter == .neverPlayed ? "Every saved song played." : "No songs."
            ) {
                SongRow(song: .placeholder)
            } row: { song in
                songLink(song, quiet: query.filter == .neverPlayed)
            }
        case .albums:
            PagedPanel(model: model.albums, query: model.albumSort, empty: "No saved albums.") {
                AlbumRow(album: .placeholder)
            } row: { album in
                albumLink(album)
            }
        case .playlists:
            PagedPanel(model: model.playlists, query: model.playlistQuery, empty: "No playlists.") {
                PlaylistRow(playlist: .placeholder)
            } row: { playlist in
                RouteLink(
                    route: .playlist(PlaylistLink(id: playlist.id, name: playlist.name)),
                    accessibilityLabel: [playlist.name, PlaylistRow.about(playlist)].joined(separator: ", "),
                    accessibilityValue: PlaylistRow.spokenValue(playlist, now: .now),
                    spotifyURI: playlist.spotifyURI
                ) {
                    PlaylistRow(playlist: playlist)
                }
            }
        }
    }

    private func songLink(_ song: SavedSong, quiet: Bool) -> some View {
        let origin = ZoomOrigin(id: "library:song:\(song.id)", namespace: zoom)
        return RouteLink(
            route: .entity(song.track, zoom: origin), accessibilityLabel: SongRow.spokenLabel(song),
            accessibilityValue: SongRow.spokenValue(song, quiet: quiet, now: .now), spotifyURI: song.spotifyURI
        ) {
            SongRow(song: song, quiet: quiet, zoom: origin)
        }
    }

    private func albumLink(_ album: SavedAlbum) -> some View {
        let origin = ZoomOrigin(id: "library:album:\(album.id)", namespace: zoom)
        return RouteLink(
            route: .entity(album.album, zoom: origin),
            accessibilityLabel: [album.album.name, album.artists].compactMap(\.self).joined(separator: ", "),
            accessibilityValue: AlbumRow.spokenValue(album, now: .now), spotifyURI: album.spotifyURI
        ) {
            AlbumRow(album: album, zoom: origin)
        }
    }

    // MARK: - State

    private var condition: LibraryCondition? {
        model.summary.map { LibraryCondition(summary: $0, status: status.systemStatus?.library) }
    }

    /// "Syncing 38%", "Synced 2 hr ago"
    private var subtitle: Text? {
        if let syncing = condition?.syncing {
            return syncing.fraction.map { Text("Syncing \(Figures.percent($0))") } ?? Text("Syncing")
        }
        guard let last = model.summary?.lastSyncAt else { return nil }
        return Text("Synced \(RelativeTime.ago(Date(milliseconds: last), now: .now))")
    }

    private var client: Client {
        session.client ?? .demo
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { LibraryTab() }
}

#Preview("Largest text") {
    PreviewHost(.demo) { LibraryTab() }
        .dynamicTypeSize(.accessibility5)
}

#Preview("Light") {
    PreviewHost(.demo) { LibraryTab() }
        .preferredColorScheme(.light)
}

#if DEBUG
/// The demo library is synced and full, so the states are drawn directly.
#Preview("States") {
    ScrollView {
        VStack(spacing: 16) {
            LibraryStatePanel(empty: .firstSync, syncing: LibraryCondition.Syncing(fraction: 0.38))
            LibraryStatePanel(empty: .firstSync, syncing: LibraryCondition.Syncing(fraction: nil))
            LibraryStatePanel(empty: .needsScope, syncing: nil)
            LibraryStatePanel(empty: .notConnected, syncing: nil)
            LibraryStatePanel(empty: .syncOff, syncing: nil)
            LibraryStatePanel(empty: .nothing, syncing: nil)
            LibraryScopeNotice()
        }
        .padding(16)
    }
    .background(.ground)
}
#endif
