import Foundation
import SpotussiAPI

/// A weighted average over time: the release year of what was played, or
/// the artists credited per track, as the web's average-dots.svelte draws
/// it. One point per bucket at its value, brighter the more of its
/// listening had the detail known; empty buckets leave a gap.
public struct AverageTrend: Hashable, Sendable {
    public enum Kind: Hashable, Sendable {
        /// "2019"
        case year
        /// "1.26"
        case ratio

        /// Ticks fall on multiples of this.
        var step: Double { self == .year ? 1 : 0.05 }
    }

    public struct Point: Identifiable, Hashable, Sendable {
        public let id: Int
        public let key: String
        /// Nil for a bucket with no listening.
        public let value: Double?
        /// The part of the bucket's listening that had the detail, 0...1.
        public let coverage: Double
    }

    public let kind: Kind
    public let bucket: ListeningSeries.Bucket
    public let points: [Point]
    /// Over the whole range; nil when nothing in it had the detail.
    public let overall: Double?

    public init(_ series: Components.Schemas.AverageSeries, kind: Kind) {
        self.kind = kind
        bucket = ListeningSeries.Bucket(rawValue: series.bucket.rawValue) ?? .day
        points = series.points.enumerated().map { index, point in
            Point(id: index, key: point.key, value: point.value, coverage: min(1, max(0, point.coverage)))
        }
        overall = series.overall
    }

    /// Points that have a value.
    public var known: [Point] { points.filter { $0.value != nil } }

    /// Runs of consecutive known points, for a line that breaks at gaps.
    public var runs: [[Point]] {
        var runs: [[Point]] = []
        var current: [Point] = []
        for point in points {
            if point.value == nil {
                if !current.isEmpty { runs.append(current) }
                current = []
            } else {
                current.append(point)
            }
        }
        if !current.isEmpty { runs.append(current) }
        return runs
    }

    /// From the first known bucket to the last, rounded: years newer (or,
    /// below zero, older) at the end, or the change in artists per track.
    public var change: Double? {
        guard let first = known.first?.value, let last = known.last?.value, known.count > 1 else { return nil }
        return kind == .year ? (last - first).rounded() : last - first
    }

    /// The scale and its ticks: a little room above and below the values,
    /// on round steps, as the web's chart picks them.
    public var scale: (domain: ClosedRange<Double>, ticks: [Double]) {
        var values = known.compactMap(\.value)
        if let overall { values.append(overall) }
        guard var low = values.min(), var high = values.max() else { return (0...1, [0, 1]) }
        let step = kind.step
        let span = max(high - low, step * 4)
        let raw = span / 3
        let magnitude = pow(10, (log10(raw / step)).rounded(.down))
        // Years skip 2.5, which would label half years.
        let steps: [Double] = kind == .year ? [1, 2, 5, 10] : [1, 2, 2.5, 5, 10]
        let tick = steps.map { $0 * step * magnitude }.first { $0 >= raw } ?? raw
        low = ((low - span * 0.1) / tick).rounded(.down) * tick
        high = ((high + span * 0.1) / tick).rounded(.up) * tick
        var ticks: [Double] = []
        var value = low
        while value <= high + tick / 2 {
            ticks.append((value * 1_000_000).rounded() / 1_000_000)
            value += tick
        }
        return (low...max(high, low + tick), ticks)
    }

    /// A value as the chart labels it.
    public func format(_ value: Double, locale: Locale = .autoupdatingCurrent) -> String {
        switch kind {
        case .year: String(Int(value.rounded()))
        case .ratio: value.formatted(.number.precision(.fractionLength(2)).locale(locale))
        }
    }
}
