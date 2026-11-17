import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

@Suite struct StatsRangeTests {
    /// `PRESET_LABEL` in web/src/lib/ranges.ts, less `custom`.
    static let webLabels: [String: String] = [
        "today": "Today", "this_week": "This week", "this_month": "This month",
        "this_year": "This year", "last_7d": "Last 7 days", "last_4w": "Last 4 weeks",
        "last_6m": "Last 6 months", "last_12m": "Last 12 months", "prev_day": "Yesterday",
        "prev_week": "Previous week", "prev_month": "Previous month",
        "prev_year": "Previous year", "all": "All time",
    ]

    @Test func everyContractPresetButCustomIsAPreset() {
        let contract = Set(Components.Schemas.RangePreset.allCases.map(\.rawValue)).subtracting(["custom"])
        #expect(Set(StatsRange.Preset.allCases.map(\.rawValue)) == contract)
        #expect(Set(Self.webLabels.keys) == contract)
    }

    /// `rangeQuery({preset})` is `{range: preset}`: the server resolves it.
    @Test(arguments: StatsRange.Preset.allCases)
    func aPresetIsSentAsRange(_ preset: StatsRange.Preset) {
        let query = StatsRange.preset(preset).query
        #expect(query.range?.rawValue == preset.rawValue)
        #expect(query.from == nil && query.to == nil)
        #expect(preset.label == Self.webLabels[preset.rawValue])
    }

    /// `rangeQuery({preset: 'custom', from, to})` is `{from, to}`, and
    /// `rangeFromParams` puts reversed dates in order.
    @Test func aCustomRangeIsSentAsFromAndTo() throws {
        let early = try #require(LocalDate("2026-01-01"))
        let late = try #require(LocalDate("2026-01-31"))
        for range in [StatsRange(from: early, to: late), StatsRange(from: late, to: early)] {
            #expect(range.query == .init(range: nil, from: "2026-01-01", to: "2026-01-31"))
            #expect(range.dayCount == 31)
            #expect(range.label == "Custom range")
        }
    }

    /// The same instant is a different day in different places.
    @Test(arguments: [
        ("Pacific/Auckland", "2026-11-17"),
        ("Europe/London", "2026-11-16"),
        ("America/Los_Angeles", "2026-11-16"),
    ])
    func customDatesComeFromTheGivenTimeZone(_ zone: String, _ expected: String) throws {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try #require(TimeZone(identifier: zone))
        let instant = try Date("2026-11-16T23:30:00Z", strategy: .iso8601)
        let range = StatsRange(from: instant, to: instant, calendar: calendar)
        #expect(range.query.from == expected)
        #expect(range.query.to == expected)
        #expect(range.compareLabel == "vs the day before")
    }

    @Test func compareLabels() throws {
        #expect(StatsRange.preset(.last4w).compareLabel == "vs the 4 weeks before")
        #expect(StatsRange.preset(.today).compareLabel == "vs the same time yesterday")
        #expect(StatsRange.preset(.all).compareLabel == nil)
        let range = StatsRange(
            from: try #require(LocalDate("2026-02-20")), to: try #require(LocalDate("2026-03-05")))
        #expect(range.compareLabel == "vs the 14 days before")
    }

    @Test func groupsAndQuickPicksMatchTheWeb() {
        #expect(StatsRange.Preset.groups.map(\.title) == ["Rolling", "So far", "Completed", "Everything"])
        #expect(Set(StatsRange.Preset.groups.flatMap(\.presets)) == Set(StatsRange.Preset.allCases))
        #expect(StatsRange.Preset.quickPicks.map(\.shortLabel) == [
            "Today", "7 days", "4 weeks", "6 months", "12 months", "All time",
        ])
        #expect(StatsRange.default == .preset(.last4w))
    }

    @Test(arguments: ["2026-1-01", "2026-02-30", "26-01-01", "2026-01-01T00:00", "", "２０２６-01-01"])
    func refusesBadDates(_ text: String) {
        #expect(LocalDate(text) == nil)
    }

    @Test(arguments: [
        StatsRange.preset(.prevMonth),
        StatsRange(from: LocalDate("2025-12-01")!, to: LocalDate("2026-01-15")!),
    ])
    func storesAndReadsBack(_ range: StatsRange) {
        #expect(StatsRange(storageValue: range.storageValue) == range)
    }
}

@MainActor @Suite struct ViewOptionsTests {
    let suite = TestDefaults()

    @Test func startsOnTheDefaults() {
        let options = ViewOptions(defaults: suite.defaults)
        #expect(options.range == .default)
        #expect(ViewOptions.Screen.allCases.allSatisfy { options.measure(for: $0) == .plays })
    }

    @Test func remembersTheRangeAndEachScreensMeasure() throws {
        let options = ViewOptions(defaults: suite.defaults)
        let range = StatsRange(from: try #require(LocalDate("2026-03-01")), to: try #require(LocalDate("2026-03-31")))
        options.range = range
        options.setMeasure(.time, for: .top)

        let relaunched = ViewOptions(defaults: suite.defaults)
        #expect(relaunched.range == range)
        #expect(relaunched.measure(for: .top) == .time)
        #expect(relaunched.measure(for: .stats) == .plays)
        #expect(relaunched.measure(for: .top).metric == .time)
    }

    @Test func ignoresAStoredValueItCannotRead() {
        suite.defaults.set("last_99y", forKey: "view.range")
        suite.defaults.set("skips", forKey: "view.measure.top")
        let options = ViewOptions(defaults: suite.defaults)
        #expect(options.range == .default)
        #expect(options.measure(for: .top) == .plays)
    }
}
