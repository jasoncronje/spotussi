import Foundation
import SpotussiAPI

/// What the Library tab says about the sync rather than the saves, derived
/// from `/library/summary` and the status's `library` as the web's Library
/// page derives it (D36).
public struct LibraryCondition: Hashable, Sendable {
    /// Why there is nothing to show.
    public enum Empty: Hashable, Sendable {
        /// `LIBRARY_SYNC_ENABLED=false` on the server.
        case syncOff
        case notConnected
        /// The Spotify connection lacks `user-library-read`.
        case needsScope
        /// The first sync hasn't finished.
        case firstSync
        case nothing
    }

    /// A sync walking the library.
    public struct Syncing: Hashable, Sendable {
        /// 0...1, when the status gives it.
        public let fraction: Double?

        public init(fraction: Double?) {
            self.fraction = fraction
        }

        /// Lamps lit out of `count` for the fraction.
        public func litLamps(of count: Int) -> Int {
            Int(((fraction ?? 0) * Double(count)).rounded())
        }
    }

    /// Nil when there are saves to show.
    public let empty: Empty?
    /// Saved songs or albums lack the permission while playlists synced:
    /// the page has rows and still misses part of the library.
    public let needsScope: Bool
    public let syncing: Syncing?

    /// `status` is nil until the instance status has loaded; an empty
    /// library then reads as nothing saved rather than guessing a cause.
    public init(summary: Components.Schemas.LibrarySummary, status: Components.Schemas.LibraryStatus?) {
        needsScope =
            status.map { $0.lists.tracks.state == .needsScope || $0.lists.albums.state == .needsScope } ?? false
        syncing =
            status.flatMap { $0.state == .syncing ? Syncing(fraction: $0.progress.map { min(1, max(0, $0)) }) : nil }
        guard summary.savedTracks + summary.savedAlbums + summary.playlists == 0 else {
            empty = nil
            return
        }
        guard let status else {
            empty = .nothing
            return
        }
        empty =
            if status.state == .disabled { .syncOff }
            else if status.state == .notConnected { .notConnected }
            else if status.lists.tracks.state == .needsScope { .needsScope }
            else if status.lastSyncAt == nil { .firstSync }
            else { .nothing }
    }
}
