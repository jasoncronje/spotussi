import Foundation
import Testing

@testable import SpotussiKit

/// The web's format.ts, in en_US. Spaces are compared plain: ICU puts
/// narrow no-break spaces before AM and PM.
@Suite struct FiguresTests {
    let us = Locale(identifier: "en_US")

    @Test func countsAreGrouped() {
        #expect(Figures.count(1_284, locale: us) == "1,284")
        #expect(Figures.count(0, locale: us) == "0")
    }

    static let minute = 60_000
    static let hour = 3_600_000
    /// Milliseconds and how the web writes them.
    static let durationCases: [(Int, String)] = [
        (0, "0s"),
        (42_400, "42s"),
        (59_600, "60s"),
        (42 * minute + 59_000, "42m"),
        (3 * hour, "3h"),
        (3 * hour + 12 * minute + 59_000, "3h 12m"),
        (56 * hour + 30 * minute, "56h 30m"),
        (136 * hour + 59 * minute, "136h"),
        (1_412 * hour, "1,412h"),
    ]

    @Test(arguments: durationCases)
    func durations(_ milliseconds: Int, _ expected: String) {
        #expect(plain(Figures.duration(milliseconds: milliseconds, locale: us)) == expected)
    }

    @Test func spokenDurationsSpellTheUnitsOut() {
        let text = Figures.duration(milliseconds: 3 * 3_600_000 + 12 * 60_000, width: .wide, locale: us)
        #expect(plain(text) == "3 hours, 12 minutes")
    }

    @Test(arguments: [
        (22_000, "0:22"),
        (221_400, "3:41"),
        (3_730_000, "1:02:10"),
    ])
    func trackLengths(_ milliseconds: Int, _ expected: String) {
        #expect(Figures.trackLength(milliseconds: milliseconds, locale: us) == expected)
    }

    // MARK: - Change

    @Test func changeRoundsLikeTheWeb() {
        // 975 against 859 is +13.5%: whole percent from 10% up.
        #expect(Change(current: 975, previous: 859) == .rise(.init(value: 14, fractionDigits: 0)))
        // Under 10%, one decimal, kept even when it is zero.
        #expect(Change(current: 100, previous: 96) == .rise(.init(value: 4.2, fractionDigits: 1)))
        #expect(Change(current: 96, previous: 100) == .fall(.init(value: 4.0, fractionDigits: 1)))
        #expect(Change(current: 1_001, previous: 100_000) == .fall(.init(value: 99, fractionDigits: 0)))
    }

    @Test func changeWithoutABaseline() {
        #expect(Change(current: 5, previous: 0) == .new)
        #expect(Change(current: 0, previous: 0) == nil)
        #expect(Change(current: 5, previous: nil) == nil)
        #expect(Change(current: 1_000, previous: 1_000) == .same)
        #expect(Change(current: 100_000, previous: 100_001) == .same)
    }

    @Test func percentText() {
        #expect(Change.Percent(value: 37, fractionDigits: 0).formatted(locale: us) == "37%")
        #expect(Change.Percent(value: 4.0, fractionDigits: 1).formatted(locale: us) == "4.0%")
        #expect(Change.Percent(value: 4.5, fractionDigits: 1).formatted(locale: us) == "4.5%")
        #expect(Change.Percent(value: 1_250, fractionDigits: 0).formatted(locale: us) == "1,250%")
    }

    // MARK: - Times

    let now = Date(timeIntervalSince1970: 1_794_908_281)  // 2026-11-17 09:38 UTC

    @Test func playTimesWithinADayAreClockTimes() {
        let text = Figures.playTime(now.addingTimeInterval(-3_600), now: now, locale: us, timeZone: .gmt)
        #expect(plain(text) == "8:38 AM")
    }

    @Test func olderPlaysAreRelative() {
        let threeDays = Figures.playTime(now.addingTimeInterval(-3 * 86_400), now: now, locale: us, timeZone: .gmt)
        #expect(threeDays == "3 days ago")
        let older = Figures.playTime(now.addingTimeInterval(-40 * 86_400), now: now, locale: us, timeZone: .gmt)
        #expect(older == "Oct 8, 2026")
    }

    @Test func timeSpansPastMidnightNameTheNextDay() {
        // Sun, Nov 8, 2026, 11:25 PM to Mon 12:52 AM, GMT.
        let start = Date(timeIntervalSince1970: 1_794_180_300)
        let end = start.addingTimeInterval(87 * 60)
        let text = plain(Figures.timeSpan(start, end, locale: us, timeZone: .gmt))
        #expect(text == "11:25 PM – Mon 12:52 AM")
        let sameDay = plain(Figures.timeSpan(start.addingTimeInterval(-3600), start, locale: us, timeZone: .gmt))
        #expect(!sameDay.contains("2026"))
    }

    @Test func agoIsShort() {
        #expect(RelativeTime.ago(now.addingTimeInterval(-20), now: now, locale: us) == "just now")
        #expect(RelativeTime.ago(now.addingTimeInterval(-86_400), now: now, locale: us) == "yesterday")
    }
}

/// Any kind of space as a plain one.
func plain(_ text: String) -> String {
    String(text.map { $0.isWhitespace ? " " : $0 })
}
