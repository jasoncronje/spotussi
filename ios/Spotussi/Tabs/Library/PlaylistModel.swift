import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// One playlist in the library and its songs in playlist order, 100 at a
/// time, as the web's playlist page.
@Observable
final class PlaylistModel {
    let id: String
    private(set) var playlist: SavedPlaylist?
    /// Deleted or unfollowed since the last sync.
    private(set) var isNotFound = false
    private(set) var isFailed = false
    /// Keyed by the playlist's id: its songs have no other query.
    let songs: PagedListModel<SavedSong, String>

    private static let pageSize = 100

    init(id: String) {
        self.id = id
        songs = PagedListModel(pageSize: Self.pageSize, fetch: SavedPlaylist.songs)
    }

    /// Loads the playlist with its first songs, unless it is on screen
    /// already (coming back from a song keeps the pages read); `force`
    /// reloads it, for pull-to-refresh.
    func load(client: Client, force: Bool = false) async {
        guard force || playlist == nil else { return }
        do {
            let detail = try await SavedPlaylist.detail(id: id, limit: Self.pageSize, client: client)
            try Task.checkCancellation()
            playlist = detail.playlist
            songs.start(detail.songs, for: id)
            isNotFound = false
            isFailed = false
        } catch {
            guard !Task.isCancelled else { return }
            isNotFound = (error as? EntityLoadError) == .notFound
            isFailed = true
            songs.fail(id)
        }
    }
}
