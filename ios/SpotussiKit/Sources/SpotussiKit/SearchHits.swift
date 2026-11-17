import Foundation
import SpotussiAPI

/// Catalog search (`/search`), as the web's entity-search.svelte runs it.
public enum CatalogSearch {
    /// The web searches from two characters on.
    public static let minimumLength = 2
    /// Results per kind, as on the web.
    public static let limit = 5
    /// The web's pause after the last keystroke.
    public static let debounce = Duration.milliseconds(160)

    /// The text to search for, trimmed; nil while it is too short to send.
    public static func query(_ text: String) -> String? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.count >= minimumLength ? String(trimmed.prefix(200)) : nil
    }
}

/// One kind of result, in the order the API returns the kinds: artists,
/// albums, tracks. Kinds with no results are left out.
public struct SearchGroup: Identifiable, Hashable, Sendable {
    public let kind: EntityKind
    public let hits: [SearchHit]

    public var id: EntityKind { kind }

    public static func list(_ results: Components.Schemas.SearchResults) -> [SearchGroup] {
        let artists = results.artists.map { artist in
            SearchHit(
                link: EntityLink(kind: .artist, id: artist.id, name: artist.name), detail: nil,
                imageURL: artist.imageUrl)
        }
        let albums = results.albums.map { album in
            SearchHit(
                link: EntityLink(kind: .album, id: album.id, name: album.name),
                detail: TopEntry.names(album.artists), imageURL: album.imageUrl)
        }
        let tracks = results.tracks.map { track in
            SearchHit(
                link: EntityLink(kind: .track, id: track.id, name: track.name),
                detail: TopEntry.names(track.artists), imageURL: track.imageUrl)
        }
        return [
            SearchGroup(kind: .artist, hits: artists),
            SearchGroup(kind: .album, hits: albums),
            SearchGroup(kind: .track, hits: tracks),
        ]
        .filter { !$0.hits.isEmpty }
    }
}

public struct SearchHit: Identifiable, Hashable, Sendable {
    public let link: EntityLink
    /// An album's or track's artists.
    public let detail: String?
    public let imageURL: String?

    public var id: String { "\(link.kind.rawValue):\(link.id)" }
}
