import Foundation
import SpotussiAPI

/// A timeline reduced to what the lamp grid draws: one column per bucket
/// (the server picks hours, days, weeks or months from the range's length),
/// each a stack of lamps lit to its share of the busiest column. Mirrors
/// timeline-chart.svelte: the previous period stands behind as unlit lamps,
/// and in Time the part estimated from track lengths is lit dimly.
public struct ListeningSeries: Hashable, Sendable {
    public enum Bucket: String, Hashable, Sendable {
        case hour, day, week, month, year
    }

    public struct Column: Identifiable, Hashable, Sendable {
        public let id: Int
        /// The bucket's key in the instance's time zone: `2026-10-17T13`,
        /// `2026-10-17`, `2026-10-12` (a week's first day), `2026-10`, `2026`.
        public let key: String
        /// Plays, or milliseconds listened.
        public let value: Int
        /// The part of `value` estimated from track lengths (Time only).
        public let estimated: Int
        /// The same position in the previous period, when there is one.
        public let previous: Int?
        public let previousKey: String?
    }

    /// How a column's lamps are split, bottom up. Whatever is left of the
    /// grid's height is empty sockets.
    public struct Lamps: Hashable, Sendable {
        /// Lit by measured listening.
        public let lit: Int
        /// Lit dimly: estimated listening above the measured part.
        public let estimated: Int
        /// Unlit, up to where the previous period reached.
        public let previous: Int
    }

    /// More columns than this can't be told apart on a phone; a long
    /// all-time series of months is shown by year instead.
    public static let maximumColumns = 72

    public let bucket: Bucket
    public let measure: Measure
    public let columns: [Column]

    public init(_ timeline: Components.Schemas.Timeline, measure: Measure) {
        let previous = timeline.previous == nil ? [] : (timeline.previousPoints ?? [])
        func value(_ point: Components.Schemas.TimelinePoint) -> Int {
            measure == .plays ? point.plays : point.msPlayed
        }
        var columns = timeline.points.enumerated().map { index, point in
            let before = previous.indices.contains(index) ? previous[index] : nil
            return Column(
                id: index, key: point.key, value: value(point),
                estimated: measure == .time ? min(point.msEstimated, point.msPlayed) : 0,
                previous: before.map(value), previousKey: before?.key)
        }
        var bucket = Bucket(rawValue: timeline.bucket.rawValue) ?? .day
        if bucket == .month, columns.count > Self.maximumColumns {
            columns = Self.byYear(columns)
            bucket = .year
        }
        self.bucket = bucket
        self.measure = measure
        self.columns = columns
    }

    public var hasPrevious: Bool { columns.contains { $0.previous != nil } }

    /// Nothing listened in any column.
    public var isEmpty: Bool { columns.allSatisfy { $0.value == 0 } }

    /// The busiest column, the first of them on a tie; nil when empty.
    public var peak: Column? {
        guard let top = columns.map(\.value).max(), top > 0 else { return nil }
        return columns.first { $0.value == top }
    }

    public var total: Int { columns.reduce(0) { $0 + $1.value } }

    public var average: Double {
        columns.isEmpty ? 0 : Double(total) / Double(columns.count)
    }

    /// What a full column stands for: the busiest column of either period.
    public var scale: Int {
        columns.reduce(0) { max($0, $1.value, $1.previous ?? 0) }
    }

    /// The lamps of `column` in a grid `rows` high. Any listening lights at
    /// least one lamp, so a quiet day never reads as none.
    public func lamps(_ column: Column, rows: Int) -> Lamps {
        let scale = scale
        func level(_ value: Int) -> Int {
            guard value > 0, scale > 0, rows > 0 else { return 0 }
            let share = Double(value) / Double(scale) * Double(rows)
            return min(rows, max(1, Int(share.rounded())))
        }
        let total = level(column.value)
        let measured = min(total, level(column.value - column.estimated))
        let previous = max(0, level(column.previous ?? 0) - total)
        return Lamps(lit: measured, estimated: total - measured, previous: previous)
    }

    /// Months summed into calendar years; the previous period's months go
    /// with the year of the month they stand behind.
    private static func byYear(_ months: [Column]) -> [Column] {
        var years: [Column] = []
        for month in months {
            let year = String(month.key.prefix(4))
            let previousYear = month.previousKey.map { String($0.prefix(4)) }
            guard let last = years.last, last.key == year else {
                years.append(
                    Column(
                        id: years.count, key: year, value: month.value, estimated: month.estimated,
                        previous: month.previous, previousKey: previousYear))
                continue
            }
            let previous: Int? =
                switch (last.previous, month.previous) {
                case (nil, nil): nil
                case (let a, let b): (a ?? 0) + (b ?? 0)
                }
            years[years.count - 1] = Column(
                id: last.id, key: year, value: last.value + month.value,
                estimated: last.estimated + month.estimated, previous: previous,
                previousKey: last.previousKey ?? previousYear)
        }
        return years
    }
}

// MARK: - Labels

extension ListeningSeries {
    /// A bucket in full, for the peak and VoiceOver: "Wed, Oct 7",
    /// "Oct 17 at 1 PM", "Oct 2026", "2026". A week is its first day,
    /// "Oct 12, 2026"; the screen says "Week of" around it.
    public func label(_ key: String, locale: Locale = .autoupdatingCurrent) -> String {
        guard let date = Self.date(key, bucket: bucket) else { return key }
        let style = Self.style(locale)
        return switch bucket {
        case .hour: date.formatted(style.month(.abbreviated).day().hour())
        case .day: date.formatted(style.weekday(.abbreviated).month(.abbreviated).day())
        case .week: date.formatted(style.month(.abbreviated).day().year())
        case .month: date.formatted(style.month(.abbreviated).year())
        case .year: key
        }
    }

    /// A bucket under the axis: "Oct 7", "1 PM", "Oct", or the year for a
    /// January.
    public func tick(_ key: String, locale: Locale = .autoupdatingCurrent) -> String {
        guard let date = Self.date(key, bucket: bucket) else { return key }
        let style = Self.style(locale)
        return switch bucket {
        case .hour: date.formatted(style.hour())
        case .day, .week: date.formatted(style.month(.abbreviated).day())
        case .month: key.hasSuffix("-01") ? String(key.prefix(4)) : date.formatted(style.month(.abbreviated))
        case .year: key
        }
    }

    /// Keys are local to the instance, so they are read and written in
    /// GMT: the day on screen is the day in the key.
    private static let calendar: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }()

    private static func style(_ locale: Locale) -> Date.FormatStyle {
        Date.FormatStyle(locale: locale, calendar: calendar, timeZone: .gmt)
    }

    private static func date(_ key: String, bucket: Bucket) -> Date? {
        switch bucket {
        case .hour:
            guard key.count >= 13, let hour = Int(key.suffix(2)),
                let day = LocalDate(String(key.prefix(10)))?.date(in: calendar)
            else { return nil }
            return calendar.date(byAdding: .hour, value: hour, to: day)
        case .day, .week:
            return LocalDate(key)?.date(in: calendar)
        case .month:
            return LocalDate(key + "-01")?.date(in: calendar)
        case .year:
            return LocalDate(key + "-01-01")?.date(in: calendar)
        }
    }
}
