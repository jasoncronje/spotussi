import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The Library tab (D36): the summary and the saves over time, and the
/// songs, albums and playlists, each a list of its own with its own sort
/// and filter. All time: saves carry their own dates, so the shared range
/// doesn't apply.
@Observable
final class LibraryModel {
    /// The three lists, as the web's tabs.
    nonisolated enum Part: Hashable, Sendable {
        case songs, albums, playlists
    }

    private(set) var summary: Components.Schemas.LibrarySummary?
    private(set) var summaryFailed = false
    private(set) var timeline: SavesTimeline?
    private(set) var timelineFailed = false

    var part = Part.songs
    /// Nil until chosen: never played first, as the web, unless every
    /// saved song has been played.
    var songFilter: SavedSong.Filter?
    var songSort = SavedSong.Sort.added
    var albumSort = SavedAlbum.Sort.added
    var playlistQuery = SavedPlaylist.Query(relation: .all, sort: .playsHere)

    let songs: PagedListModel<SavedSong, SavedSong.Query>
    let albums: PagedListModel<SavedAlbum, SavedAlbum.Sort>
    let playlists: PagedListModel<SavedPlaylist, SavedPlaylist.Query>

    // Made here rather than as default values: a default value can't hold
    // a loader that runs off the main actor.
    init() {
        songs = PagedListModel(fetch: SavedSong.page)
        albums = PagedListModel(fetch: SavedAlbum.page)
        playlists = PagedListModel(fetch: SavedPlaylist.page)
    }

    /// The songs to list; nil until the summary says which filter leads.
    var songsQuery: SavedSong.Query? {
        let leading = summary.map { summary -> SavedSong.Filter in
            summary.tracks.neverPlayed == 0 ? .all : .neverPlayed
        }
        guard let filter = songFilter ?? leading else { return nil }
        return SavedSong.Query(filter: filter, sort: songSort)
    }

    /// Loads the summary and the saves over time together; each keeps its
    /// last answer when a reload fails.
    func load(client: Client) async {
        async let summary = try? client.getLibrarySummary().ok.body.json
        async let timeline = try? SavesTimeline.load(client: client)
        let (newSummary, newTimeline) = await (summary, timeline)
        // Cancelled: the screen went away, and the next visit loads again.
        guard !Task.isCancelled else { return }
        if let newSummary { self.summary = newSummary }
        summaryFailed = newSummary == nil
        if let newTimeline { self.timeline = newTimeline }
        timelineFailed = newTimeline == nil
    }

    /// Loads the part on screen, unless it is already; `force` reloads it.
    func loadPart(client: Client, force: Bool = false) async {
        switch part {
        case .songs:
            if let query = songsQuery { await songs.load(query, client: client, force: force) }
        case .albums:
            await albums.load(albumSort, client: client, force: force)
        case .playlists:
            await playlists.load(playlistQuery, client: client, force: force)
        }
    }

    /// What the lists depend on; a change loads the part on screen.
    nonisolated struct Shown: Hashable, Sendable {
        let part: Part
        let songs: SavedSong.Query?
        let albums: SavedAlbum.Sort
        let playlists: SavedPlaylist.Query
    }

    var shown: Shown {
        Shown(part: part, songs: songsQuery, albums: albumSort, playlists: playlistQuery)
    }
}
