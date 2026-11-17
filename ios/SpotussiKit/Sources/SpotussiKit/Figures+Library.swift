import Foundation

/// Days and dates as the Library writes them.
extension Figures {
    /// A median number of days: one decimal under 10 ("2.5", "3"), whole
    /// days from there ("266"), a dash when there is none, as the web's
    /// Library readouts.
    public static func days(_ median: Double?, locale: Locale = .autoupdatingCurrent) -> String {
        guard let median else { return "–" }
        if median < 10 {
            return median.formatted(.number.precision(.fractionLength(0...1)).locale(locale))
        }
        return count(Int(median.rounded()), locale: locale)
    }

    /// A day without the weekday: "Oct 14", with the year when it isn't
    /// this one ("Oct 14, 2024").
    public static func date(
        _ date: Date, now: Date, calendar: Calendar = .current, locale: Locale = .autoupdatingCurrent
    ) -> String {
        let style = Date.FormatStyle(locale: locale, calendar: calendar, timeZone: calendar.timeZone)
            .month(.abbreviated).day()
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        return date.formatted(sameYear ? style : style.year())
    }
}
