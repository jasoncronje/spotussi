import Foundation
import SpotussiAPI

/// The period a view covers: a preset the server resolves in the instance's
/// time zone, or a custom span of local dates. Mirrors `RangeSel` in
/// web/src/lib/ranges.ts.
public enum StatsRange: Hashable, Sendable {
    case preset(Preset)
    /// Inclusive local dates, `from` never after `to`; build one with
    /// `init(from:to:)`, which puts them in order.
    case custom(from: LocalDate, to: LocalDate)

    /// The contract default when no range is given.
    public static let `default` = StatsRange.preset(.last4w)

    /// A custom range, in order whichever way round the dates come.
    public init(from: LocalDate, to: LocalDate) {
        self = from <= to ? .custom(from: from, to: to) : .custom(from: to, to: from)
    }

    /// A custom range from two instants, read as days in `calendar`'s time zone.
    public init(from: Date, to: Date, calendar: Calendar = .current) {
        self.init(from: LocalDate(from, calendar: calendar), to: LocalDate(to, calendar: calendar))
    }

    public enum Preset: String, CaseIterable, Hashable, Sendable {
        case today
        case thisWeek = "this_week"
        case thisMonth = "this_month"
        case thisYear = "this_year"
        case last7d = "last_7d"
        case last4w = "last_4w"
        case last6m = "last_6m"
        case last12m = "last_12m"
        case prevDay = "prev_day"
        case prevWeek = "prev_week"
        case prevMonth = "prev_month"
        case prevYear = "prev_year"
        case all
    }

    /// The API's query parameters: `range` for a preset, `from` and `to` for
    /// a custom range. Pass them straight to an operation's `Query`.
    public var query: Query {
        switch self {
        case .preset(let preset): Query(range: preset.apiValue, from: nil, to: nil)
        case .custom(let from, let to): Query(range: nil, from: from.description, to: to.description)
        }
    }

    public struct Query: Hashable, Sendable {
        public let range: Components.Schemas.RangePreset?
        public let from: String?
        public let to: String?
    }

    public var label: String {
        switch self {
        case .preset(let preset): preset.label
        case .custom: String(localized: "Custom range", bundle: .module)
        }
    }

    /// Days covered by a custom range, counting both ends; nil for presets.
    public var dayCount: Int? {
        guard case .custom(let from, let to) = self else { return nil }
        return from.days(to: to) + 1
    }

    /// What the change figures compare against: "vs the 4 weeks before".
    /// Nil for all time, which has nothing before it.
    public var compareLabel: String? {
        switch self {
        case .preset(let preset):
            return preset.compareLabel
        case .custom:
            let days = dayCount ?? 0
            return days == 1
                ? String(localized: "vs the day before", bundle: .module)
                : String(localized: "vs the \(days) days before", bundle: .module)
        }
    }
}

// MARK: - Presets

extension StatsRange.Preset {
    public var apiValue: Components.Schemas.RangePreset {
        Components.Schemas.RangePreset(rawValue: rawValue)!
    }

    public var label: String {
        switch self {
        case .today: String(localized: "Today", bundle: .module)
        case .thisWeek: String(localized: "This week", bundle: .module)
        case .thisMonth: String(localized: "This month", bundle: .module)
        case .thisYear: String(localized: "This year", bundle: .module)
        case .last7d: String(localized: "Last 7 days", bundle: .module)
        case .last4w: String(localized: "Last 4 weeks", bundle: .module)
        case .last6m: String(localized: "Last 6 months", bundle: .module)
        case .last12m: String(localized: "Last 12 months", bundle: .module)
        case .prevDay: String(localized: "Yesterday", bundle: .module)
        case .prevWeek: String(localized: "Previous week", bundle: .module)
        case .prevMonth: String(localized: "Previous month", bundle: .module)
        case .prevYear: String(localized: "Previous year", bundle: .module)
        case .all: String(localized: "All time", bundle: .module)
        }
    }

    /// For tight spots such as a toolbar button. The web's quick picks have
    /// their own short labels; the rest are short already.
    public var shortLabel: String {
        switch self {
        case .last7d: String(localized: "7 days", bundle: .module)
        case .last4w: String(localized: "4 weeks", bundle: .module)
        case .last6m: String(localized: "6 months", bundle: .module)
        case .last12m: String(localized: "12 months", bundle: .module)
        default: label
        }
    }

    /// The web's segmented quick picks, in order.
    public static let quickPicks: [Self] = [.today, .last7d, .last4w, .last6m, .last12m, .all]

    /// Every preset, grouped the way people think about them.
    public static var groups: [Group] {
        [
            Group(
                title: String(localized: "Rolling", bundle: .module),
                presets: [.last7d, .last4w, .last6m, .last12m]),
            Group(
                title: String(localized: "So far", bundle: .module),
                presets: [.today, .thisWeek, .thisMonth, .thisYear]),
            Group(
                title: String(localized: "Completed", bundle: .module),
                presets: [.prevDay, .prevWeek, .prevMonth, .prevYear]),
            Group(title: String(localized: "Everything", bundle: .module), presets: [.all]),
        ]
    }

    public struct Group: Hashable, Sendable, Identifiable {
        public let title: String
        public let presets: [StatsRange.Preset]
        public var id: String { title }
    }

    var compareLabel: String? {
        switch self {
        case .today: String(localized: "vs the same time yesterday", bundle: .module)
        case .thisWeek: String(localized: "vs the same point last week", bundle: .module)
        case .thisMonth: String(localized: "vs the same point last month", bundle: .module)
        case .thisYear: String(localized: "vs the same point last year", bundle: .module)
        case .last7d: String(localized: "vs the 7 days before", bundle: .module)
        case .last4w: String(localized: "vs the 4 weeks before", bundle: .module)
        case .last6m: String(localized: "vs the 6 months before", bundle: .module)
        case .last12m: String(localized: "vs the 12 months before", bundle: .module)
        case .prevDay: String(localized: "vs the day before", bundle: .module)
        case .prevWeek: String(localized: "vs the week before", bundle: .module)
        case .prevMonth: String(localized: "vs the month before", bundle: .module)
        case .prevYear: String(localized: "vs the year before", bundle: .module)
        case .all: nil
        }
    }
}

// MARK: - Storage

extension StatsRange {
    /// `last_4w`, or `2026-01-01/2026-01-31` for a custom range.
    var storageValue: String {
        switch self {
        case .preset(let preset): preset.rawValue
        case .custom(let from, let to): "\(from)/\(to)"
        }
    }

    init?(storageValue: String) {
        if let preset = Preset(rawValue: storageValue) {
            self = .preset(preset)
            return
        }
        let parts = storageValue.split(separator: "/")
        guard parts.count == 2, let from = LocalDate(String(parts[0])),
            let to = LocalDate(String(parts[1]))
        else { return nil }
        self.init(from: from, to: to)
    }
}

// MARK: - Local dates

/// A calendar day with no time zone, as the API's `from` and `to` take it:
/// `2026-11-17`.
public struct LocalDate: Hashable, Comparable, Sendable, CustomStringConvertible {
    public let year: Int
    public let month: Int
    public let day: Int

    /// Reads `yyyy-MM-dd`; anything else, or a day that doesn't exist, is nil.
    public init?(_ text: String) {
        let parts = text.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0].count == 4, parts[1].count == 2, parts[2].count == 2,
            parts.allSatisfy({ $0.allSatisfy(\.isASCII) && $0.allSatisfy(\.isNumber) }),
            let year = Int(parts[0]), let month = Int(parts[1]), let day = Int(parts[2])
        else { return nil }
        var components = DateComponents(year: year, month: month, day: day)
        components.calendar = Self.gregorian
        guard components.isValidDate else { return nil }
        self.year = year
        self.month = month
        self.day = day
    }

    /// The day `date` falls on in `calendar`'s time zone.
    public init(_ date: Date, calendar: Calendar = .current) {
        let parts = calendar.dateComponents([.year, .month, .day], from: date)
        year = parts.year!
        month = parts.month!
        day = parts.day!
    }

    /// Midnight at the start of the day in `calendar`'s time zone.
    public func date(in calendar: Calendar = .current) -> Date? {
        calendar.date(from: DateComponents(year: year, month: month, day: day))
    }

    public var description: String {
        String(format: "%04d-%02d-%02d", year, month, day)
    }

    public static func < (lhs: Self, rhs: Self) -> Bool {
        (lhs.year, lhs.month, lhs.day) < (rhs.year, rhs.month, rhs.day)
    }

    func days(to other: LocalDate) -> Int {
        guard let start = date(in: Self.gregorian), let end = other.date(in: Self.gregorian) else {
            return 0
        }
        return Self.gregorian.dateComponents([.day], from: start, to: end).day ?? 0
    }

    private static let gregorian: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }()
}

// MARK: - Measure

/// What rankings and charts count: counted plays or time listened (D7).
public enum Measure: String, CaseIterable, Hashable, Sendable {
    case plays
    case time

    public var metric: Components.Schemas.Metric {
        switch self {
        case .plays: .plays
        case .time: .time
        }
    }

    public var label: String {
        switch self {
        case .plays: String(localized: "Plays", bundle: .module)
        case .time: String(localized: "Time", bundle: .module)
        }
    }
}
