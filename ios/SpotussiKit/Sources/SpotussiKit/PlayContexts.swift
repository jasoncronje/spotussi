import Foundation
import SpotussiAPI

/// Where the listening was played from: playlists, albums, artist pages,
/// Liked Songs, as the web's context-list.svelte lists them. Context is
/// known only for plays captured live (D2, D8); the rest is `unknown`.
public struct PlayContexts: Hashable, Sendable {
    public let items: [PlayContext]
    public let total: Int
    /// Listening with no known context: plays from an imported export.
    public let unknownPlays: Int
    public let unknownMilliseconds: Int

    public init(_ contexts: Components.Schemas.Contexts) {
        items = contexts.items.map(PlayContext.init)
        total = contexts.total
        unknownPlays = contexts.unknown.plays
        unknownMilliseconds = contexts.unknown.msPlayed
    }

    public func unknown(for measure: Measure) -> Int {
        measure == .plays ? unknownPlays : unknownMilliseconds
    }

    /// Nothing to list in the range.
    public func isEmpty(for measure: Measure) -> Bool {
        items.isEmpty && unknown(for: measure) == 0
    }

    /// What a full meter stands for: the busiest context.
    public func leader(for measure: Measure) -> Int {
        max(1, items.map { $0.value(for: measure) }.max() ?? 1)
    }
}

public struct PlayContext: Identifiable, Hashable, Sendable {
    public enum Kind: Hashable, Sendable {
        case playlist, album, artist, likedSongs, other
    }

    /// The Spotify URI.
    public let id: String
    public let rank: Int
    public let kind: Kind
    /// Nil when unresolved, such as a Spotify playlist nobody has named.
    public let name: String?
    /// Made by Spotify, which apps can't read (D17).
    public let isSpotifyPlaylist: Bool
    /// Named by hand on the web.
    public let isNamedByOwner: Bool
    public let imageURL: String?
    public let plays: Int
    public let milliseconds: Int

    public init(_ ranked: Components.Schemas.RankedContext) {
        let context = ranked.context
        id = context.uri
        rank = ranked.rank
        kind =
            switch context._type {
            case .playlist: .playlist
            case .album: .album
            case .artist: .artist
            case .collection: .likedSongs
            case .other: .other
            }
        name = context.name
        isSpotifyPlaylist = context.spotifyOwned
        isNamedByOwner = context.nameSource == .manual
        imageURL = context.imageUrl
        plays = ranked.plays
        milliseconds = ranked.msPlayed
    }

    public func value(for measure: Measure) -> Int {
        measure == .plays ? plays : milliseconds
    }

    /// The library's id for a playlist context: its URI's last part, as
    /// the server builds `spotify:playlist:<id>`. Nil for other kinds.
    public var playlistID: String? {
        guard kind == .playlist else { return nil }
        let parts = id.split(separator: ":", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0] == "spotify", parts[1] == "playlist", !parts[2].isEmpty else { return nil }
        return String(parts[2])
    }
}
