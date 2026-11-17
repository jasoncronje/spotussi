import Foundation
import SpotussiAPI

/// One row of a top list, whatever kind it ranks, as the web's top-list
/// adapters in routes/+page.svelte shape it.
public struct TopEntry: Identifiable, Hashable, Sendable {
    public typealias Kind = EntityKind

    public let kind: Kind
    /// The entity's API id, for opening its page.
    public let entityID: String
    public let rank: Int
    public let name: String
    /// A track's or album's artists, "A, B".
    public var detail: String?
    public let imageURL: String?
    public let plays: Int
    public let milliseconds: Int
    /// Nil when the range has no previous period to move from.
    public let movement: Movement?
    /// `spotify:track:…`; nil for genres, local files and artists or albums
    /// known only from an export.
    public let spotifyURI: String?

    public init(
        kind: Kind, entityID: String, rank: Int, name: String, detail: String?, imageURL: String?,
        plays: Int, milliseconds: Int, movement: Movement?, spotifyURI: String? = nil
    ) {
        self.kind = kind
        self.entityID = entityID
        self.rank = rank
        self.name = name
        self.detail = detail
        self.imageURL = imageURL
        self.plays = plays
        self.milliseconds = milliseconds
        self.movement = movement
        self.spotifyURI = spotifyURI
    }

    public var id: String { "\(kind.rawValue):\(entityID)" }

    public var link: EntityLink { EntityLink(kind: kind, id: entityID, name: name) }

    public func value(for measure: Measure) -> Int {
        measure == .plays ? plays : milliseconds
    }

    /// Plays or time listened, as the row shows it: "36", "2h 5m".
    public func figure(for measure: Measure, locale: Locale = .autoupdatingCurrent) -> String {
        switch measure {
        case .plays: Figures.count(plays, locale: locale)
        case .time: Figures.duration(milliseconds: milliseconds, locale: locale)
        }
    }
}

/// Rank against the previous period, as the web's movement.svelte.
public enum Movement: Hashable, Sendable {
    /// Unranked the period before.
    case new
    case same
    /// Places gained or lost, and the rank before.
    case up(Int, from: Int)
    case down(Int, from: Int)

    public init?(rank: Int, previousRank: Int?, hasPrevious: Bool) {
        guard hasPrevious else { return nil }
        guard let previousRank else {
            self = .new
            return
        }
        let gained = previousRank - rank
        self =
            if gained > 0 { .up(gained, from: previousRank) }
            else if gained < 0 { .down(-gained, from: previousRank) }
            else { .same }
    }
}

// MARK: - From the API

extension TopEntry {
    public static func list(_ top: Components.Schemas.TopArtists, limit: Int = 5) -> [TopEntry] {
        top.items.prefix(limit).map { item in
            TopEntry(
                kind: .artist, entityID: item.artist.id, rank: item.rank, name: item.artist.name,
                detail: nil, imageURL: item.artist.imageUrl, plays: item.plays,
                milliseconds: item.msPlayed,
                movement: Movement(rank: item.rank, previousRank: item.prevRank, hasPrevious: top.previous != nil),
                spotifyURI: item.artist.uri)
        }
    }

    public static func list(_ top: Components.Schemas.TopTracks, limit: Int = 5) -> [TopEntry] {
        top.items.prefix(limit).map { item in
            TopEntry(
                kind: .track, entityID: item.track.id, rank: item.rank, name: item.track.name,
                detail: names(item.track.artists), imageURL: item.track.imageUrl, plays: item.plays,
                milliseconds: item.msPlayed,
                movement: Movement(rank: item.rank, previousRank: item.prevRank, hasPrevious: top.previous != nil),
                spotifyURI: item.track.uri)
        }
    }

    public static func list(_ top: Components.Schemas.TopAlbums, limit: Int = 5) -> [TopEntry] {
        top.items.prefix(limit).map { item in
            TopEntry(
                kind: .album, entityID: item.album.id, rank: item.rank, name: item.album.name,
                detail: names(item.album.artists), imageURL: item.album.imageUrl, plays: item.plays,
                milliseconds: item.msPlayed,
                movement: Movement(rank: item.rank, previousRank: item.prevRank, hasPrevious: top.previous != nil),
                spotifyURI: item.album.uri)
        }
    }

    public static func list(_ top: Components.Schemas.TopGenres, limit: Int = 5) -> [TopEntry] {
        top.items.prefix(limit).map { item in
            TopEntry(
                kind: .genre, entityID: item.genre.id, rank: item.rank, name: item.genre.name,
                detail: nil, imageURL: nil, plays: item.plays, milliseconds: item.msPlayed,
                movement: Movement(rank: item.rank, previousRank: item.prevRank, hasPrevious: top.previous != nil))
        }
    }

    /// The web joins names with a comma, whatever the count.
    static func names(_ artists: [Components.Schemas.ArtistLite]) -> String? {
        artists.isEmpty ? nil : artists.map(\.name).joined(separator: ", ")
    }
}

// MARK: - Recent plays

/// One play in the recent list, as recent-plays.svelte shows it.
public struct RecentPlay: Identifiable, Hashable, Sendable {
    /// The honest detail under the time.
    public enum Note: Hashable, Sendable {
        /// Skipped this far in.
        case skipped(atMilliseconds: Int)
        /// Too short to count as a play.
        case notCounted
        /// Length estimated from the track's length.
        case estimated(milliseconds: Int)
    }

    public let id: String
    public let trackID: String
    public let name: String
    public let artists: String?
    public let imageURL: String?
    public let playedAt: Date
    public let note: Note?
    /// When it stopped; the start when unknown.
    public let endedAt: Date
    /// Passes the play rule (D7).
    public let isCounted: Bool
    public let isSkipped: Bool

    public init(
        id: String, trackID: String, name: String, artists: String?, imageURL: String?,
        playedAt: Date, note: Note?, endedAt: Date? = nil, isCounted: Bool = true, isSkipped: Bool = false
    ) {
        self.id = id
        self.trackID = trackID
        self.name = name
        self.artists = artists
        self.imageURL = imageURL
        self.playedAt = playedAt
        self.note = note
        self.endedAt = endedAt ?? playedAt
        self.isCounted = isCounted
        self.isSkipped = isSkipped
    }

    /// A row of the API's play log.
    public init(_ play: Components.Schemas.Play) {
        let note: Note? =
            if play.skipped { .skipped(atMilliseconds: play.msPlayed) }
            else if !play.countsAsPlay { .notCounted }
            else if play.msEstimated { .estimated(milliseconds: play.msPlayed) }
            else { nil }
        self.init(
            id: play.id, trackID: play.track.id, name: play.track.name,
            artists: TopEntry.names(play.track.artists), imageURL: play.track.imageUrl,
            playedAt: Date(milliseconds: play.playedAt), note: note, endedAt: Date(milliseconds: play.endedAt),
            isCounted: play.countsAsPlay, isSkipped: play.skipped)
    }

    public static func list(_ history: Components.Schemas.History, limit: Int = 10) -> [RecentPlay] {
        history.items.prefix(limit).map(RecentPlay.init)
    }
}

extension Date {
    /// An instant from the API's milliseconds since 1970.
    public init(milliseconds: some BinaryInteger) {
        self.init(timeIntervalSince1970: Double(milliseconds) / 1000)
    }
}
