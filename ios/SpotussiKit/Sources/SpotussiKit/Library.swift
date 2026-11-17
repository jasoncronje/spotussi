import Foundation
import SpotussiAPI

/// A saved song, or a song in a playlist, as the web's song-row shows it:
/// when it was saved or added, its plays as rotation lamps, and the play
/// that took it into heavy rotation, or why it has none (D36).
public struct SavedSong: Identifiable, Hashable, Sendable {
    public typealias Status = Components.Schemas.LibraryPlayStatus
    public typealias Filter = Components.Parameters.LibraryPlayed
    public typealias Sort = Operations.GetLibraryTracks.Input.Query.SortPayload

    /// Which saved songs, in what order.
    public struct Query: Hashable, Sendable {
        public var filter: Filter
        public var sort: Sort

        public init(filter: Filter, sort: Sort) {
            self.filter = filter
            self.sort = sort
        }
    }

    /// Who put a song in a collaborative playlist.
    public enum Adder: Hashable, Sendable {
        case you
        /// A collaborator's Spotify user ID.
        case other(String)
    }

    /// How long after saving a song reached heavy rotation.
    public enum Lag: Hashable, Sendable {
        case beforeSaving
        case sameDay
        case days(Int)
    }

    /// The line under a song's date.
    public enum Note: Hashable, Sendable {
        case neverPlayed
        /// Never played as this release, but as another with the same
        /// title and artist.
        case otherRelease
        /// Reached heavy rotation; the lag is nil when the save date isn't
        /// known.
        case rotation(Lag?)
        case lastPlayed(Date)
    }

    public let id: String
    public let track: EntityLink
    /// "A, B"
    public let artists: String?
    public let imageURL: String?
    public let spotifyURI: String?
    public let status: Status
    /// Counted plays of the song, from anywhere.
    public let plays: Int
    /// Saved, or added to the playlist.
    public let addedAt: Date?
    public let lastPlayedAt: Date?
    /// The counted play that made it heavy rotation.
    public let rotationAt: Date?
    /// In a playlist: its place, counting from 0.
    public let position: Int?
    /// In a playlist: counted plays started from that playlist.
    public let playsHere: Int?
    /// In a collaborative playlist: who added it.
    public let adder: Adder?

    public init(_ saved: Components.Schemas.LibraryTrack) {
        let added = saved.addedAt.map(String.init) ?? ""
        id = "\(saved.track.id)@\(added)"
        track = EntityLink(kind: .track, id: saved.track.id, name: saved.track.name)
        artists = TopEntry.names(saved.track.artists)
        imageURL = saved.track.imageUrl
        spotifyURI = saved.track.uri
        status = saved.status
        plays = saved.plays
        addedAt = saved.addedAt.map(Date.init(milliseconds:))
        lastPlayedAt = saved.lastPlayedAt.map(Date.init(milliseconds:))
        rotationAt = saved.rotationAt.map(Date.init(milliseconds:))
        position = nil
        playsHere = nil
        adder = nil
    }

    /// A playlist's song; who added it is told for collaborative playlists
    /// only, as on the web.
    public init(_ item: Components.Schemas.LibraryPlaylistItem, collaborative: Bool) {
        id = "\(item.position)"
        track = EntityLink(kind: .track, id: item.track.id, name: item.track.name)
        artists = TopEntry.names(item.track.artists)
        imageURL = item.track.imageUrl
        spotifyURI = item.track.uri
        status = item.status
        plays = item.plays
        addedAt = item.addedAt.map(Date.init(milliseconds:))
        lastPlayedAt = nil
        rotationAt = nil
        position = item.position
        playsHere = item.playsHere
        if collaborative, let by = item.addedBy {
            adder = item.addedByOwner ? .you : .other(by)
        } else {
            adder = nil
        }
    }

    /// The counted play that marks heavy rotation (D36).
    public static let rotationPlays = 5

    /// Lamps lit, one per counted play up to heavy rotation.
    public func litLamps(of count: Int = rotationPlays) -> Int {
        max(0, min(plays, count))
    }

    /// What to say under the date. `quiet` leaves out "never played" in a
    /// list of only those.
    public func note(quiet: Bool = false) -> Note? {
        switch status {
        case .neverPlayed: return quiet ? nil : .neverPlayed
        case .maybePlayed: return .otherRelease
        case .played: break
        }
        if let rotationAt {
            return .rotation(addedAt.map { Self.lag(from: $0, to: rotationAt) })
        }
        return lastPlayedAt.map(Note.lastPlayed)
    }

    static func lag(from added: Date, to rotation: Date) -> Lag {
        let days = Int((rotation.timeIntervalSince(added) / 86_400).rounded())
        if days < 0 { return .beforeSaving }
        return days == 0 ? .sameDay : .days(days)
    }

    /// One page of saved songs.
    public static func page(
        _ query: Query, offset: Int, limit: Int, client: Client
    ) async throws -> OffsetPage<SavedSong> {
        let answer = try await client.getLibraryTracks(
            query: .init(played: query.filter, sort: query.sort, limit: limit, offset: offset)
        ).ok.body.json
        return OffsetPage(items: answer.items.map(SavedSong.init), offset: answer.offset, total: answer.total)
    }
}

/// How many of an album's or playlist's songs were ever played.
public struct PlayThrough: Hashable, Sendable {
    public let played: Int
    public let total: Int

    /// Nil when there is nothing to count against.
    public init?(played: Int, total: Int?) {
        guard let total, total > 0 else { return nil }
        self.played = max(0, min(played, total))
        self.total = total
    }

    public var share: Double { Double(played) / Double(total) }

    /// A row of dots as the web's share-dots: one per song while they fit
    /// in `limit`, else a share of the row; `lit` is how many are lit,
    /// with a part-lit last one.
    public func dots(limit: Int = 24) -> (count: Int, lit: Double) {
        let count = max(1, min(total, limit))
        return (count, share * Double(count))
    }
}

/// A saved album with its plays and how many of its tracks were played.
public struct SavedAlbum: Identifiable, Hashable, Sendable {
    public typealias Sort = Operations.GetLibraryAlbums.Input.Query.SortPayload

    public let album: EntityLink
    public let artists: String?
    public let imageURL: String?
    public let spotifyURI: String?
    public let addedAt: Date?
    public let plays: Int
    public let milliseconds: Int
    public let tracksPlayed: Int
    /// Nil when the album's track count isn't known.
    public let playThrough: PlayThrough?

    public var id: String { album.id }

    public init(_ saved: Components.Schemas.LibraryAlbum) {
        album = EntityLink(kind: .album, id: saved.album.id, name: saved.album.name)
        artists = TopEntry.names(saved.album.artists)
        imageURL = saved.album.imageUrl
        spotifyURI = saved.album.uri
        addedAt = saved.addedAt.map(Date.init(milliseconds:))
        plays = saved.plays
        milliseconds = saved.msPlayed
        tracksPlayed = saved.tracksPlayed
        playThrough = PlayThrough(played: saved.tracksPlayed, total: saved.tracksTotal)
    }

    public static func page(
        _ sort: Sort, offset: Int, limit: Int, client: Client
    ) async throws -> OffsetPage<SavedAlbum> {
        let answer = try await client.getLibraryAlbums(
            query: .init(sort: sort, limit: limit, offset: offset)
        ).ok.body.json
        return OffsetPage(items: answer.items.map(SavedAlbum.init), offset: answer.offset, total: answer.total)
    }
}

/// A playlist in the library: whose it is, its songs when Spotify shares
/// them (own and collaborative playlists), and the plays started from it.
public struct SavedPlaylist: Identifiable, Hashable, Sendable {
    public typealias Relation = Components.Schemas.LibraryPlaylistRef.RelationPayload
    public typealias RelationFilter = Operations.GetLibraryPlaylists.Input.Query.RelationPayload
    public typealias Sort = Operations.GetLibraryPlaylists.Input.Query.SortPayload

    /// Which playlists, in what order.
    public struct Query: Hashable, Sendable {
        public var relation: RelationFilter
        public var sort: Sort

        public init(relation: RelationFilter, sort: Sort) {
            self.relation = relation
            self.sort = sort
        }
    }

    public let id: String
    public let name: String
    public let relation: Relation
    public let ownerName: String?
    public let imageURL: String?
    public let spotifyURI: String
    /// Items as Spotify counts them, episodes included.
    public let itemsTotal: Int?
    /// Spotify shares its songs: own and collaborative playlists.
    public let itemsAvailable: Bool
    /// Different songs ever played, of the different songs in it now.
    public let playThrough: PlayThrough?
    /// Counted plays started from it, all time.
    public let playsHere: Int
    public let millisecondsHere: Int
    public let lastPlayedHereAt: Date?

    public init(_ saved: Components.Schemas.LibraryPlaylist) {
        id = saved.playlist.id
        name = saved.playlist.name
        relation = saved.playlist.relation
        ownerName = saved.playlist.ownerName
        imageURL = saved.playlist.imageUrl
        spotifyURI = saved.playlist.uri
        itemsTotal = saved.itemsTotal
        itemsAvailable = saved.itemsAvailable
        playThrough =
            saved.itemsAvailable ? PlayThrough(played: saved.tracksPlayed ?? 0, total: saved.tracks) : nil
        playsHere = saved.playsHere
        millisecondsHere = saved.msHere
        lastPlayedHereAt = saved.lastPlayedHereAt.map(Date.init(milliseconds:))
    }

    public static func page(
        _ query: Query, offset: Int, limit: Int, client: Client
    ) async throws -> OffsetPage<SavedPlaylist> {
        let answer = try await client.getLibraryPlaylists(
            query: .init(relation: query.relation, sort: query.sort, limit: limit, offset: offset)
        ).ok.body.json
        return OffsetPage(items: answer.items.map(SavedPlaylist.init), offset: answer.offset, total: answer.total)
    }

    /// A playlist and a page of its songs in playlist order. Throws
    /// ``EntityLoadError/notFound`` when it isn't in the library (deleted
    /// or unfollowed since the last sync).
    public static func detail(
        id: String, offset: Int = 0, limit: Int, client: Client
    ) async throws -> (playlist: SavedPlaylist, songs: OffsetPage<SavedSong>) {
        switch try await client.getLibraryPlaylist(path: .init(id: id), query: .init(limit: limit, offset: offset)) {
        case .ok(let ok):
            let detail = try ok.body.json
            let collaborative = detail.playlist.playlist.relation == .collaborative
            let songs = detail.items.map { SavedSong($0, collaborative: collaborative) }
            return (SavedPlaylist(detail.playlist), OffsetPage(items: songs, offset: detail.offset, total: detail.total))
        case .notFound: throw EntityLoadError.notFound
        case .default(let status, _): throw EntityLoadError.status(status)
        }
    }

    /// A later page of a playlist's songs.
    public static func songs(
        id: String, offset: Int, limit: Int, client: Client
    ) async throws -> OffsetPage<SavedSong> {
        try await detail(id: id, offset: offset, limit: limit, client: client).songs
    }
}
