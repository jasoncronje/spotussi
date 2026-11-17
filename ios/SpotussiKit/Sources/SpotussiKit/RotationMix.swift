import Foundation
import SpotussiAPI

/// When saved songs reached heavy rotation (their 5th counted play), as the
/// web's save-lag mix meter: a row of dots split into runs by part.
public struct RotationMix: Hashable, Sendable {
    public enum Part: CaseIterable, Hashable, Sendable {
        case beforeSaving, withinDay, withinWeek, withinMonth, later
        /// Fewer plays than heavy rotation so far.
        case notYet
    }

    /// Songs per part, in `Part.allCases` order.
    public let counts: [Int]

    public init(counts: [Int]) {
        precondition(counts.count == Part.allCases.count)
        self.counts = counts.map { max(0, $0) }
    }

    public init(_ mix: Components.Schemas.LibrarySummary.RotationMixPayload) {
        self.init(counts: [
            mix.beforeSaving, mix.withinDay, mix.withinWeek, mix.withinMonth, mix.later, mix.notYet,
        ])
    }

    public var total: Int { counts.reduce(0, +) }

    public func count(_ part: Part) -> Int {
        counts[Part.allCases.firstIndex(of: part)!]
    }

    public func share(_ part: Part) -> Double {
        total == 0 ? 0 : Double(count(part)) / Double(total)
    }

    /// The parts with songs in them.
    public var shown: [Part] { Part.allCases.filter { count($0) > 0 } }

    /// Dots per part, in order, adding up to `dots`: largest remainder, and
    /// any part with songs gets at least one, as the web's mix meter.
    public func runs(dots: Int = 30) -> [Int] {
        let total = total
        guard total > 0 else { return counts.map { _ in 0 } }
        let exact = counts.map { Double($0) / Double(total) * Double(dots) }
        var runs = zip(counts, exact).map { count, exact in count > 0 ? max(1, Int(exact)) : 0 }
        var left = dots - runs.reduce(0, +)
        let order = exact.indices.sorted { a, b in
            let ra = exact[a] - exact[a].rounded(.down)
            let rb = exact[b] - exact[b].rounded(.down)
            return ra == rb ? a < b : ra > rb
        }
        var step = 0
        while left > 0, step < order.count * 2 {
            let index = order[step % order.count]
            if counts[index] > 0 {
                runs[index] += 1
                left -= 1
            }
            step += 1
        }
        while left < 0, let index = runs.indices.max(by: { runs[$0] < runs[$1] }) {
            runs[index] -= 1
            left += 1
        }
        return runs
    }

    /// "46%", or "<1%" for a sliver that would round to nothing.
    public static func percent(_ share: Double, locale: Locale = .autoupdatingCurrent) -> String {
        if share > 0, share < 0.01 {
            return "<" + Figures.percent(0.01, locale: locale)
        }
        return Figures.percent(share, locale: locale)
    }
}
