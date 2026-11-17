import Foundation
import SpotussiAPI

/// The head of an artist, album, track or genre page: who or what it is,
/// where it ranks in the range, its totals in the range and all time, and
/// the details the API knows, as the web's entity-view.svelte shows them.
public struct EntityDetail: Hashable, Sendable {
    /// What the page calls the entity above its name.
    public enum Kicker: Hashable, Sendable {
        case artist, album, single, compilation, track, genre
    }

    /// Plays, time and skips over some period.
    public struct Tally: Hashable, Sendable {
        public let plays: Int
        public let milliseconds: Int
        public let skips: Int

        public func value(for measure: Measure) -> Int {
            measure == .plays ? plays : milliseconds
        }
    }

    public let link: EntityLink
    public let kicker: Kicker
    public let imageURL: String?
    /// `spotify:artist:…`; nil for genres and for what Spotify doesn't know.
    public let spotifyURI: String?
    /// Left out of every stat on the server; plays stay recorded.
    public let isExcluded: Bool

    /// Rank among entities of the same kind by the measure; nil when not
    /// listened in the range.
    public let rank: Int?
    /// How many entities of this kind ranked in the range.
    public let rankedTotal: Int
    public let inRange: Tally
    public let allTime: Tally
    public let firstPlayed: Date?
    public let lastPlayed: Date?

    /// An artist's genres.
    public let genres: [EntityLink]
    /// An album's or track's artists, in credit order.
    public let artists: [EntityLink]
    /// A track's album.
    public let album: EntityLink?
    /// An album's release date, as precise as Spotify knows it: `2014`,
    /// `2014-12` or `2014-12-11`.
    public let releaseDate: String?
    public let trackCount: Int?
    /// A track's length.
    public let duration: Int?
    public let isExplicit: Bool
    /// When a track was saved to the library, if it is saved now (D36).
    public let savedAt: Date?
    /// Artists in the catalog with a genre.
    public let artistCount: Int?

    public var name: String { link.name }

    /// Anything was listened in the range.
    public var isListened: Bool { inRange.plays > 0 || inRange.milliseconds > 0 }

    /// The page for `kind` and `id`, in `range`, ranked by `measure`.
    /// Throws ``EntityLoadError/notFound`` when the instance has no such
    /// entity.
    public static func load(
        _ kind: EntityKind, id: String, range: StatsRange, measure: Measure, client: Client
    ) async throws -> EntityDetail {
        let query = range.query
        let metric = measure.metric
        switch kind {
        case .artist:
            switch try await client.getArtist(
                path: .init(id: id), query: .init(range: query.range, from: query.from, to: query.to, metric: metric))
            {
            case .ok(let ok): return EntityDetail(try ok.body.json)
            case .notFound: throw EntityLoadError.notFound
            case .default(let status, _): throw EntityLoadError.status(status)
            }
        case .album:
            switch try await client.getAlbum(
                path: .init(id: id), query: .init(range: query.range, from: query.from, to: query.to, metric: metric))
            {
            case .ok(let ok): return EntityDetail(try ok.body.json)
            case .notFound: throw EntityLoadError.notFound
            case .default(let status, _): throw EntityLoadError.status(status)
            }
        case .track:
            switch try await client.getTrack(
                path: .init(id: id), query: .init(range: query.range, from: query.from, to: query.to, metric: metric))
            {
            case .ok(let ok): return EntityDetail(try ok.body.json)
            case .notFound: throw EntityLoadError.notFound
            case .default(let status, _): throw EntityLoadError.status(status)
            }
        case .genre:
            switch try await client.getGenre(
                path: .init(id: id), query: .init(range: query.range, from: query.from, to: query.to, metric: metric))
            {
            case .ok(let ok): return EntityDetail(try ok.body.json)
            case .notFound: throw EntityLoadError.notFound
            case .default(let status, _): throw EntityLoadError.status(status)
            }
        }
    }
}

public enum EntityLoadError: Error, Equatable, Sendable {
    /// No entity with this id on the instance.
    case notFound
    /// Any other answer than the page, with its status.
    case status(Int)
}

// MARK: - From the API

extension EntityDetail {
    public init(_ detail: Components.Schemas.ArtistDetail) {
        let artist = detail.artist
        self.init(
            link: EntityLink(kind: .artist, id: artist.id, name: artist.name), kicker: .artist,
            imageURL: artist.imageUrl, spotifyURI: artist.uri, isExcluded: detail.excluded,
            stats: detail.inRange, allTime: detail.allTime,
            genres: artist.genres.map { EntityLink(kind: .genre, id: $0.id, name: $0.name) })
    }

    public init(_ detail: Components.Schemas.AlbumDetail) {
        let album = detail.album
        let kicker: Kicker =
            switch album.albumType {
            case .single: .single
            case .compilation: .compilation
            case .album, ._empty_, nil: .album
            }
        self.init(
            link: EntityLink(kind: .album, id: album.id, name: album.name), kicker: kicker,
            imageURL: album.imageUrl, spotifyURI: album.uri, isExcluded: false,
            stats: detail.inRange, allTime: detail.allTime,
            artists: album.artists.map { EntityLink(kind: .artist, id: $0.id, name: $0.name) },
            releaseDate: album.releaseDate ?? album.releaseYear.map(String.init),
            trackCount: album.totalTracks)
    }

    public init(_ detail: Components.Schemas.TrackDetail) {
        let track = detail.track
        self.init(
            link: EntityLink(kind: .track, id: track.id, name: track.name), kicker: .track,
            imageURL: track.imageUrl, spotifyURI: track.uri, isExcluded: detail.excluded,
            stats: detail.inRange, allTime: detail.allTime,
            artists: track.artists.map { EntityLink(kind: .artist, id: $0.id, name: $0.name) },
            album: track.album.map { EntityLink(kind: .album, id: $0.id, name: $0.name) },
            duration: track.durationMs, isExplicit: track.explicit ?? false,
            savedAt: detail.savedAt.map(Self.date))
    }

    public init(_ detail: Components.Schemas.GenreDetail) {
        let genre = detail.genre
        self.init(
            link: EntityLink(kind: .genre, id: genre.id, name: genre.name), kicker: .genre,
            imageURL: nil, spotifyURI: nil, isExcluded: false,
            stats: detail.inRange, allTime: detail.allTime, artistCount: detail.artists)
    }

    private init(
        link: EntityLink, kicker: Kicker, imageURL: String?, spotifyURI: String?, isExcluded: Bool,
        stats: Components.Schemas.EntityStats, allTime: Components.Schemas.AllTimeStats,
        genres: [EntityLink] = [], artists: [EntityLink] = [], album: EntityLink? = nil,
        releaseDate: String? = nil, trackCount: Int? = nil, duration: Int? = nil, isExplicit: Bool = false,
        savedAt: Date? = nil, artistCount: Int? = nil
    ) {
        self.link = link
        self.kicker = kicker
        self.imageURL = imageURL
        self.spotifyURI = spotifyURI
        self.isExcluded = isExcluded
        rank = stats.rank
        rankedTotal = stats.rankedTotal
        inRange = Tally(plays: stats.plays, milliseconds: stats.msPlayed, skips: stats.skips)
        self.allTime = Tally(plays: allTime.plays, milliseconds: allTime.msPlayed, skips: allTime.skips)
        firstPlayed = allTime.firstPlayedAt.map(Self.date)
        lastPlayed = allTime.lastPlayedAt.map(Self.date)
        self.genres = genres
        self.artists = artists
        self.album = album
        self.releaseDate = releaseDate
        self.trackCount = trackCount
        self.duration = duration
        self.isExplicit = isExplicit
        self.savedAt = savedAt
        self.artistCount = artistCount
    }

    private static func date(_ milliseconds: Int64) -> Date {
        Date(timeIntervalSince1970: Double(milliseconds) / 1000)
    }
}
