import Foundation

/// What a page can be about (D14).
public enum EntityKind: String, CaseIterable, Hashable, Sendable {
    case artist, track, album, genre
}

/// An artist, album, track or genre to open, with the name to show until
/// its page has loaded.
public struct EntityLink: Hashable, Sendable {
    public let kind: EntityKind
    public let id: String
    public let name: String

    public init(kind: EntityKind, id: String, name: String) {
        self.kind = kind
        self.id = id
        self.name = name
    }
}

/// Spotify's own addresses for its URIs.
public enum SpotifyLink {
    /// `spotify:track:ID` as `https://open.spotify.com/track/ID`. The web
    /// address opens the Spotify app when it is installed and the web
    /// player when it isn't, where the `spotify:` URI would do nothing.
    public static func url(for uri: String) -> URL? {
        let parts = uri.split(separator: ":", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0] == "spotify", !parts[1].isEmpty, !parts[2].isEmpty,
            parts[1].allSatisfy({ $0.isASCII && $0.isLetter }),
            parts[2].allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber) })
        else { return nil }
        return URL(string: "https://open.spotify.com/\(parts[1])/\(parts[2])")
    }
}
