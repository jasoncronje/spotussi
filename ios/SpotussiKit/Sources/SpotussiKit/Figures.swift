import Foundation

/// Numbers as the web writes them (web/src/lib/format.ts), through the
/// system's format styles so they follow the phone's locale.
public enum Figures {
    /// "1,284"
    public static func count(_ value: Int, locale: Locale = .autoupdatingCurrent) -> String {
        value.formatted(.number.locale(locale))
    }

    /// Listening time: "42s", "42m", "3h 12m", "136h". Like the web's
    /// `durationParts`, minutes are rounded down and dropped from 100 hours
    /// on, where they stop meaning much. `.wide` spells the units out for
    /// VoiceOver.
    public static func duration(
        milliseconds: Int, width: Duration.UnitsFormatStyle.UnitWidth = .narrow,
        locale: Locale = .autoupdatingCurrent
    ) -> String {
        let minute = 60_000
        let hour = 60 * minute
        let (units, rounding): (Set<Duration.UnitsFormatStyle.Unit>, FloatingPointRoundingRule) =
            switch milliseconds {
            case ..<minute: ([.seconds], .toNearestOrAwayFromZero)
            case ..<hour: ([.minutes], .down)
            case (100 * hour)...: ([.hours], .down)
            default: ([.hours, .minutes], .down)
            }
        return Duration.milliseconds(max(0, milliseconds))
            .formatted(
                .units(allowed: units, width: width, fractionalPart: .hide(rounded: rounding))
                    .locale(locale))
    }

    /// A track's length or a position in it: "3:41", "1:02:10".
    public static func trackLength(milliseconds: Int, locale: Locale = .autoupdatingCurrent) -> String {
        let pattern: Duration.TimeFormatStyle.Pattern =
            milliseconds >= 3_600_000 ? .hourMinuteSecond : .minuteSecond
        return Duration.milliseconds(max(0, milliseconds))
            .formatted(Duration.TimeFormatStyle(pattern: pattern, locale: locale))
    }

    /// A release date as precisely as it is known: "Dec 11, 2014",
    /// "Dec 2014" or "2014". Nil for anything that isn't one of the API's
    /// three shapes.
    public static func releaseDate(_ text: String, locale: Locale = .autoupdatingCurrent) -> String? {
        let parts = text.split(separator: "-", omittingEmptySubsequences: false).map { Int($0) }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        let base = Date.FormatStyle(locale: locale, calendar: calendar, timeZone: .gmt)
        switch parts.count {
        case 1:
            guard let year = parts[0], text.count == 4 else { return nil }
            return String(year)
        case 2:
            guard let year = parts[0], let month = parts[1], (1...12).contains(month),
                let date = calendar.date(from: DateComponents(year: year, month: month, day: 1))
            else { return nil }
            return date.formatted(base.year().month(.abbreviated))
        case 3:
            guard let day = LocalDate(text), let date = day.date(in: calendar) else { return nil }
            return date.formatted(base.year().month(.abbreviated).day())
        default:
            return nil
        }
    }

    /// When a recent play started: the clock time within the last day, and
    /// "3 days ago" or a date before that, like the web's recent plays.
    public static func playTime(
        _ date: Date, now: Date, locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        guard now.timeIntervalSince(date) >= 86_400 else {
            return date.formatted(
                Date.FormatStyle(date: .omitted, time: .shortened, locale: locale, timeZone: timeZone))
        }
        return RelativeTime.ago(date, now: now, locale: locale)
    }
}

/// "3 min ago", "yesterday", or a date for anything more than a week away,
/// like `ago()` in web/src/lib/format.ts.
public enum RelativeTime {
    public static func ago(_ date: Date, now: Date, locale: Locale = .autoupdatingCurrent) -> String {
        let seconds = date.timeIntervalSince(now).rounded()
        let magnitude = abs(seconds)
        if magnitude < 45 { return String(localized: "just now", bundle: .module, locale: locale) }
        if magnitude >= 7 * 86_400 {
            return date.formatted(Date.FormatStyle(date: .abbreviated, time: .omitted, locale: locale))
        }
        let formatter = RelativeDateTimeFormatter()
        formatter.locale = locale
        formatter.dateTimeStyle = .named
        formatter.unitsStyle = .short
        let components =
            if magnitude < 3600 {
                DateComponents(minute: Int((seconds / 60).rounded()))
            } else if magnitude < 86_400 {
                DateComponents(hour: Int((seconds / 3600).rounded()))
            } else {
                DateComponents(day: Int((seconds / 86_400).rounded()))
            }
        return formatter.localizedString(from: components)
    }
}

// MARK: - Change

/// A figure against the period before, rounded as the web's `delta()`:
/// one decimal under 10%, whole percent from there.
public enum Change: Hashable, Sendable {
    /// Something this period, nothing the period before.
    case new
    /// Rounds to 0%.
    case same
    case rise(Percent)
    case fall(Percent)

    /// Nil when there is nothing to compare: no previous period, or
    /// nothing in either.
    public init?(current: Int, previous: Int?) {
        guard let previous else { return nil }
        guard previous != 0 else {
            if current == 0 { return nil }
            self = .new
            return
        }
        let ratio = Double(current - previous) / Double(previous)
        let magnitude = abs(ratio * 100)
        let digits = magnitude < 10 && magnitude != 0 ? 1 : 0
        let scale = digits == 1 ? 10.0 : 1.0
        let rounded = (magnitude * scale).rounded(.toNearestOrAwayFromZero) / scale
        if rounded == 0 {
            self = .same
            return
        }
        let percent = Percent(value: rounded, fractionDigits: digits)
        self = ratio > 0 ? .rise(percent) : .fall(percent)
    }

    /// A rounded percentage and the decimals it was rounded to.
    public struct Percent: Hashable, Sendable {
        public let value: Double
        public let fractionDigits: Int

        public init(value: Double, fractionDigits: Int) {
            self.value = value
            self.fractionDigits = fractionDigits
        }

        /// "37%", "4.5%"
        public func formatted(locale: Locale = .autoupdatingCurrent) -> String {
            (value / 100).formatted(
                .percent.precision(.fractionLength(fractionDigits)).locale(locale))
        }
    }
}
