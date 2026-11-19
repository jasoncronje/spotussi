import Foundation

/// Hours, clock times and days, as the web's format.ts writes them.
extension Figures {
    /// An hour of the day: "10 AM", "22".
    public static func hour(_ hour: Int, locale: Locale = .autoupdatingCurrent) -> String {
        hourDate(hour).formatted(gmtStyle(locale).hour())
    }

    /// The hour as a span, for VoiceOver and the clock's readout:
    /// "10 – 11 AM".
    public static func hourSpan(_ hour: Int, locale: Locale = .autoupdatingCurrent) -> String {
        let start = hourDate(hour)
        let end = start.addingTimeInterval(3600)
        return (start..<end).formatted(
            Date.IntervalFormatStyle(locale: locale, calendar: gmt, timeZone: .gmt).hour())
    }

    /// A clock time: "8:19 AM".
    public static func clockTime(
        _ date: Date, locale: Locale = .autoupdatingCurrent, timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        date.formatted(Date.FormatStyle(date: .omitted, time: .shortened, locale: locale, timeZone: timeZone))
    }

    /// From one clock time to another: "2:46 – 4:26 PM", or past midnight
    /// "11:25 PM – Mon 12:52 AM". The interval style would write both full
    /// dates ("11/8/2026, 11:25 PM – 11/9/2026, 12:52 AM") under a heading
    /// that already names the day.
    public static func timeSpan(
        _ start: Date, _ end: Date, locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        guard end > start else { return clockTime(start, locale: locale, timeZone: timeZone) }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        if !calendar.isDate(start, inSameDayAs: end) {
            let endStyle = Date.FormatStyle(locale: locale, calendar: calendar, timeZone: timeZone)
                .weekday(.abbreviated).hour().minute()
            return "\(clockTime(start, locale: locale, timeZone: timeZone)) – \(end.formatted(endStyle))"
        }
        return (start..<end).formatted(
            Date.IntervalFormatStyle(locale: locale, calendar: calendar, timeZone: timeZone).hour().minute())
    }

    /// A day heading a list: "Today", "Yesterday", "Wed, Oct 21", and the
    /// year too when it isn't this one.
    public static func day(
        _ date: Date, now: Date, calendar: Calendar = .current, locale: Locale = .autoupdatingCurrent
    ) -> String {
        if calendar.isDate(date, inSameDayAs: now) {
            return String(localized: "Today", bundle: .module, locale: locale)
        }
        if let yesterday = calendar.date(byAdding: .day, value: -1, to: now),
            calendar.isDate(date, inSameDayAs: yesterday)
        {
            return String(localized: "Yesterday", bundle: .module, locale: locale)
        }
        let style = Date.FormatStyle(locale: locale, calendar: calendar, timeZone: calendar.timeZone)
            .weekday(.abbreviated).month(.abbreviated).day()
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        return date.formatted(sameYear ? style : style.year())
    }

    /// A share as a whole percentage: "38%".
    public static func percent(_ share: Double, locale: Locale = .autoupdatingCurrent) -> String {
        share.formatted(.percent.precision(.fractionLength(0)).locale(locale))
    }

    // Hours of the day carry no date or zone: they are drawn on a fixed
    // day in GMT so "10" is always 10.
    private static let gmt: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }()

    private static func hourDate(_ hour: Int) -> Date {
        let base = gmt.date(from: DateComponents(year: 2000, month: 1, day: 1))!
        return base.addingTimeInterval(Double((hour % 24 + 24) % 24) * 3600)
    }

    private static func gmtStyle(_ locale: Locale) -> Date.FormatStyle {
        Date.FormatStyle(locale: locale, calendar: gmt, timeZone: .gmt)
    }
}
