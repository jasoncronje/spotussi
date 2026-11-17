import Foundation
import SpotussiAPI

/// A continuous run of listening, split where the silence between two rows
/// is longer than the instance's session gap, as the web's Sessions.
public struct ListeningSession: Identifiable, Hashable, Sendable {
    public let id: String
    public let startedAt: Date
    public let endedAt: Date
    /// From the first start to the last end.
    public let duration: Int
    /// Milliseconds actually listened.
    public let listened: Int
    public let rows: Int
    /// Counted plays.
    public let plays: Int
    public let skips: Int
    /// Most played, by time, at most three.
    public let topArtists: [EntityLink]
    public let artistImageURLs: [String?]

    public init(_ session: Components.Schemas.Session) {
        id = session.id
        startedAt = Date(milliseconds: session.startedAt)
        endedAt = Date(milliseconds: session.endedAt)
        duration = session.durationMs
        listened = session.msPlayed
        rows = session.rows
        plays = session.plays
        skips = session.skips
        topArtists = session.topArtists.prefix(3).map { EntityLink(kind: .artist, id: $0.id, name: $0.name) }
        artistImageURLs = session.topArtists.prefix(3).map(\.imageUrl)
    }

    /// Time inside the session with nothing playing.
    public var silence: Int { max(0, duration - listened) }

    /// "Bitter Parade, DJ Elny, The Midnight Lanterns"
    public var artistNames: String? {
        topArtists.isEmpty ? nil : topArtists.map(\.name).joined(separator: ", ")
    }

    /// A session and its rows in order. Throws ``EntityLoadError/notFound``
    /// when the instance has no such session: sessions are worked out again
    /// when the gap or the history changes.
    public static func load(id: String, client: Client) async throws -> (ListeningSession, [RecentPlay]) {
        switch try await client.getSession(path: .init(id: id)) {
        case .ok(let ok):
            let detail = try ok.body.json
            return (ListeningSession(detail.session), detail.plays.map(RecentPlay.init))
        case .notFound: throw EntityLoadError.notFound
        case .default(let status, _): throw EntityLoadError.status(status)
        }
    }
}

/// The longest sessions in a range, longest first, paged by offset with no
/// cap.
public struct SessionList: Hashable, Sendable {
    public private(set) var sessions: [ListeningSession] = []
    public private(set) var total: Int
    /// Minutes of silence that end a session.
    public let gapMinutes: Int
    public private(set) var nextOffset = 0
    public private(set) var isComplete = false
    private var seen: Set<String> = []

    public static let pageSize = 25

    public init(first page: Components.Schemas.Sessions) {
        total = page.total
        gapMinutes = page.gapMinutes
        append(page, requestedAt: 0)
    }

    /// The first, longest session's length: what a full meter stands for.
    public var longest: Int { max(1, sessions.first?.duration ?? 1) }

    /// Adds the page asked for at `offset`, as ``RankedList`` does: a page
    /// that starts elsewhere or brings nothing new ends the list.
    public mutating func append(_ page: Components.Schemas.Sessions, requestedAt offset: Int) {
        let fresh = page.items.filter { seen.insert($0.id).inserted }.map(ListeningSession.init)
        sessions += fresh
        total = page.total
        nextOffset = page.offset + page.items.count
        isComplete = fresh.isEmpty || page.offset != offset || nextOffset >= page.total
    }

    public static func load(
        range: StatsRange, offset: Int = 0, limit: Int = pageSize, client: Client
    ) async throws -> Components.Schemas.Sessions {
        let span = range.query
        return try await client.getSessions(
            query: .init(range: span.range, from: span.from, to: span.to, limit: limit, offset: offset)
        ).ok.body.json
    }
}

/// A session's rows placed at their real times between its first start
/// and last end, as the tape on the web's session page: counted plays lit,
/// skipped or uncounted rows unlit, silence as gaps.
public struct SessionTape: Hashable, Sendable {
    public struct Mark: Identifiable, Hashable, Sendable {
        public let id: String
        /// 0...1 across the session.
        public let start: Double
        public let end: Double
        public let isLit: Bool
    }

    public let marks: [Mark]

    public init(session: ListeningSession, plays: [RecentPlay]) {
        let origin = session.startedAt.timeIntervalSince1970
        let span = max(1, session.endedAt.timeIntervalSince1970 - origin)
        func position(_ date: Date) -> Double {
            min(1, max(0, (date.timeIntervalSince1970 - origin) / span))
        }
        marks = plays.map { play in
            let start = position(play.playedAt)
            return Mark(
                id: play.id, start: start, end: max(start, position(play.endedAt)),
                isLit: play.isCounted && !play.isSkipped)
        }
    }
}
