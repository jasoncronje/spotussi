import Foundation
import SpotussiKit

extension StatsRange {
    /// For the toolbar: "4 weeks", or the dates of a custom range.
    var shortLabel: String {
        switch self {
        case .preset(let preset): preset.shortLabel
        case .custom(let from, let to): Self.interval(from, to) ?? label
        }
    }

    /// For subtitles and VoiceOver: "Last 4 weeks", or the dates.
    var longLabel: String {
        switch self {
        case .preset(let preset): preset.label
        case .custom(let from, let to): Self.interval(from, to) ?? label
        }
    }

    var isCustom: Bool {
        if case .custom = self { true } else { false }
    }

    /// "Oct 20 – Nov 16, 2026", or one day on its own.
    static func interval(_ from: LocalDate, _ to: LocalDate) -> String? {
        guard let start = from.date(), let end = to.date() else { return nil }
        if from == to { return start.formatted(date: .abbreviated, time: .omitted) }
        return (start..<end).formatted(.interval.day().month(.abbreviated).year())
    }

    /// The same, from the API's `yyyy-MM-dd` strings.
    static func interval(_ from: String, _ to: String) -> String? {
        guard let from = LocalDate(from), let to = LocalDate(to) else { return nil }
        return interval(from, to)
    }
}
