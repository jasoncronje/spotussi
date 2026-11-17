import Foundation
import SpotussiAPI

/// One page of a top list, as the API answered it.
public struct TopPage: Hashable, Sendable {
    public let entries: [TopEntry]
    /// Where the server says this page starts.
    public let offset: Int
    /// Ranked entries in the whole list.
    public let total: Int
    /// The range has a period before it, so rows carry movement.
    public let hasPrevious: Bool

    public init(entries: [TopEntry], offset: Int, total: Int, hasPrevious: Bool) {
        self.entries = entries
        self.offset = offset
        self.total = total
        self.hasPrevious = hasPrevious
    }

    public init(_ top: Components.Schemas.TopTracks) {
        self.init(
            entries: TopEntry.list(top, limit: .max), offset: top.offset, total: top.total,
            hasPrevious: top.previous != nil)
    }

    public init(_ top: Components.Schemas.TopArtists) {
        self.init(
            entries: TopEntry.list(top, limit: .max), offset: top.offset, total: top.total,
            hasPrevious: top.previous != nil)
    }

    public init(_ top: Components.Schemas.TopAlbums) {
        self.init(
            entries: TopEntry.list(top, limit: .max), offset: top.offset, total: top.total,
            hasPrevious: top.previous != nil)
    }

    public init(_ top: Components.Schemas.TopGenres) {
        self.init(
            entries: TopEntry.list(top, limit: .max), offset: top.offset, total: top.total,
            hasPrevious: top.previous != nil)
    }

    /// A page of one kind's ranking, optionally narrowed to the plays of one
    /// entity, as the web's top lists and entity pages ask for it. A scope
    /// the kind can't be narrowed by (genres by anything, artists by an
    /// artist) is left out, as the API allows one scope at most.
    public static func load(
        _ kind: EntityKind, within scope: EntityLink? = nil, range: StatsRange, measure: Measure,
        offset: Int = 0, limit: Int, client: Client
    ) async throws -> TopPage {
        let query = range.query
        let metric = measure.metric
        func scoped(_ kind: EntityKind) -> String? {
            scope?.kind == kind ? scope?.id : nil
        }
        switch kind {
        case .track:
            return TopPage(
                try await client.getTopTracks(
                    query: .init(
                        range: query.range, from: query.from, to: query.to, metric: metric, limit: limit,
                        offset: offset, artistId: scoped(.artist), albumId: scoped(.album),
                        genreId: scoped(.genre))
                ).ok.body.json)
        case .artist:
            return TopPage(
                try await client.getTopArtists(
                    query: .init(
                        range: query.range, from: query.from, to: query.to, metric: metric, limit: limit,
                        offset: offset, albumId: scoped(.album), genreId: scoped(.genre))
                ).ok.body.json)
        case .album:
            return TopPage(
                try await client.getTopAlbums(
                    query: .init(
                        range: query.range, from: query.from, to: query.to, metric: metric, limit: limit,
                        offset: offset, artistId: scoped(.artist), genreId: scoped(.genre))
                ).ok.body.json)
        case .genre:
            return TopPage(
                try await client.getTopGenres(
                    query: .init(
                        range: query.range, from: query.from, to: query.to, metric: metric, limit: limit,
                        offset: offset)
                ).ok.body.json)
        }
    }
}

/// A ranking as far as it has been paged, with no cap on its length (D14).
/// Pages are asked for by offset; the list is complete when the server has
/// nothing more to give.
public struct RankedList: Hashable, Sendable {
    public private(set) var entries: [TopEntry]
    public private(set) var total: Int
    public let hasPrevious: Bool
    public private(set) var isComplete = false
    /// Where the next page starts.
    public private(set) var nextOffset = 0
    private var seen: Set<String> = []

    public init(first page: TopPage) {
        entries = []
        total = page.total
        hasPrevious = page.hasPrevious
        append(page, requestedAt: 0)
    }

    /// What a full meter stands for: the first entry's value, over the
    /// whole ranking, as the web's ranked table measures it.
    public func leader(for measure: Measure) -> Int {
        max(1, entries.first?.value(for: measure) ?? 1)
    }

    /// Adds the page asked for at `offset`. Entries already listed are
    /// dropped, in case the ranking moved between two pages.
    public mutating func append(_ page: TopPage, requestedAt offset: Int) {
        let fresh = page.entries.filter { seen.insert($0.id).inserted }
        entries += fresh
        total = page.total
        nextOffset = page.offset + page.entries.count
        // A server that ignores the offset answers with the first page again
        // (the demo data does): stop there rather than repeat it for ever.
        isComplete = fresh.isEmpty || page.offset != offset || nextOffset >= page.total
    }
}
