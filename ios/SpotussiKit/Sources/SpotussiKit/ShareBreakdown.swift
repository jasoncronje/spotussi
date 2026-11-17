import Foundation
import SpotussiAPI

/// The share of each bucket held by a few artists or genres, as the web's
/// share-columns.svelte stacks it: one segment per series from the bottom,
/// the rest of the column unlit. The scale tops out just above the busiest
/// column rather than at 100%, as a few series rarely hold most of the
/// listening and thin slivers would be unreadable.
public struct ShareBreakdown: Hashable, Sendable {
    public struct Series: Identifiable, Hashable, Sendable {
        /// Position in the stack and the legend, 0 at the bottom.
        public let id: Int
        public let link: EntityLink
        /// Its part of the whole range, 0...1.
        public let share: Double
    }

    public struct Column: Identifiable, Hashable, Sendable {
        public let id: Int
        public let key: String
        /// The bucket's plays or milliseconds.
        public let total: Int
        /// Each series' part of the bucket, 0...1, in series order.
        public let shares: [Double]
        /// What no series holds.
        public var other: Double { total > 0 ? max(0, 1 - shares.reduce(0, +)) : 0 }
    }

    /// One series' part of one column, from `start` to `end` on the scale.
    public struct Segment: Identifiable, Hashable, Sendable {
        public let id: Int
        public let series: Int
        public let start: Double
        public let end: Double
    }

    public let bucket: ListeningSeries.Bucket
    public let measure: Measure
    public let series: [Series]
    public let columns: [Column]

    /// Series a phone can tell apart without a rainbow: one lit, four
    /// steps of aluminium.
    public static let maximumSeries = 5

    public init(_ artists: Components.Schemas.ArtistsOverTime, measure: Measure) {
        self.init(
            bucket: artists.bucket.rawValue,
            links: artists.series.map { EntityLink(kind: .artist, id: $0.id, name: $0.name) },
            points: artists.points, measure: measure)
    }

    public init(_ genres: Components.Schemas.GenresOverTime, measure: Measure) {
        self.init(
            bucket: genres.bucket.rawValue,
            links: genres.series.map { EntityLink(kind: .genre, id: $0.id, name: $0.name) },
            points: genres.points, measure: measure)
    }

    init(bucket: String, links: [EntityLink], points: [Components.Schemas.BreakdownPoint], measure: Measure) {
        let links = Array(links.prefix(Self.maximumSeries))
        var bucket = ListeningSeries.Bucket(rawValue: bucket) ?? .day
        var rows = points.map { point in
            (key: point.key, total: point.total, values: Array(point.values.prefix(links.count)))
        }
        // Like the lamp grid, a long run of months is shown by year; totals
        // and values add up across months.
        if bucket == .month, rows.count > ListeningSeries.maximumColumns {
            var years: [(key: String, total: Int, values: [Int])] = []
            for row in rows {
                let year = String(row.key.prefix(4))
                if let last = years.last, last.key == year {
                    years[years.count - 1].total += row.total
                    years[years.count - 1].values = zip(last.values, row.values).map(+)
                } else {
                    years.append((year, row.total, row.values))
                }
            }
            rows = years
            bucket = .year
        }
        let whole = rows.reduce(0) { $0 + $1.total }
        series = links.enumerated().map { index, link in
            let held = rows.reduce(0) { $0 + ($1.values.indices.contains(index) ? $1.values[index] : 0) }
            return Series(id: index, link: link, share: whole > 0 ? Double(held) / Double(whole) : 0)
        }
        columns = rows.enumerated().map { index, row in
            let shares = (0..<links.count).map { position in
                guard row.total > 0, row.values.indices.contains(position) else { return 0.0 }
                return Double(row.values[position]) / Double(row.total)
            }
            return Column(id: index, key: row.key, total: row.total, shares: shares)
        }
        self.bucket = bucket
        self.measure = measure
    }

    /// The series' parts of `column`, stacked from the bottom.
    public func segments(_ column: Column) -> [Segment] {
        var start = 0.0
        return column.shares.enumerated().compactMap { index, share in
            defer { start += share }
            guard share > 0 else { return nil }
            return Segment(id: index, series: index, start: start, end: start + share)
        }
    }

    /// The top of the scale: the busiest column's stacked share, rounded up
    /// to 5%, at least 5% and at most 100%.
    public var scale: Double {
        let busiest = columns.map { $0.shares.reduce(0, +) }.max() ?? 0
        return min(1, max(0.05, (busiest * 20 + 0.0001).rounded(.up) / 20))
    }

    /// What all the series hold together over the range.
    public var held: Double { series.reduce(0) { $0 + $1.share } }

    /// The range's listening, in plays or milliseconds.
    public var total: Int { columns.reduce(0) { $0 + $1.total } }
}
