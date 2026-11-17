import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

@Suite struct ListeningSeriesTests {
    let us = Locale(identifier: "en_US")

    @Test func theDemoTimelineIsFourWeeksOfDays() throws {
        let timeline = try JSONDecoder().decode(
            Components.Schemas.Timeline.self, from: Fixtures.data(for: "getTimeline")!)
        let series = ListeningSeries(timeline, measure: .plays)
        #expect(series.bucket == .day)
        #expect(series.columns.count == 28)
        #expect(series.hasPrevious)
        #expect(series.total == timeline.points.reduce(0) { $0 + $1.plays })
        let peak = try #require(series.peak)
        #expect(peak.value == timeline.points.map(\.plays).max())
        #expect(series.columns.allSatisfy { $0.estimated == 0 })
    }

    @Test func lampsSplitIntoLitAndThePeriodBefore() throws {
        let series = ListeningSeries(
            try timeline(.day, values: [0, 5, 10, 1], previous: [10, 0, 0, 0]), measure: .plays)
        #expect(series.scale == 10)
        let lamps = series.columns.map { series.lamps($0, rows: 8) }
        #expect(lamps[0] == .init(lit: 0, estimated: 0, previous: 8))
        #expect(lamps[1] == .init(lit: 4, estimated: 0, previous: 0))
        #expect(lamps[2] == .init(lit: 8, estimated: 0, previous: 0))
        // Any listening lights a lamp.
        #expect(lamps[3] == .init(lit: 1, estimated: 0, previous: 0))
        #expect(series.peak?.key == "2026-10-03")
        #expect(series.average == 4)
    }

    @Test func estimatedTimeIsLitDimly() throws {
        let series = ListeningSeries(
            try timeline(.day, values: [1_000, 400], estimated: [500, 0]), measure: .time)
        let lamps = series.columns.map { series.lamps($0, rows: 8) }
        #expect(lamps[0] == .init(lit: 4, estimated: 4, previous: 0))
        #expect(lamps[1] == .init(lit: 3, estimated: 0, previous: 0))
        #expect(!series.hasPrevious)
    }

    @Test func playsIgnoreEstimates() throws {
        let series = ListeningSeries(
            try timeline(.day, values: [1_000], estimated: [500]), measure: .plays)
        #expect(series.columns[0].estimated == 0)
    }

    @Test func anEmptyRangeLightsNothing() throws {
        let series = ListeningSeries(try timeline(.day, values: [0, 0, 0]), measure: .plays)
        #expect(series.isEmpty)
        #expect(series.peak == nil)
        #expect(series.lamps(series.columns[0], rows: 8) == .init(lit: 0, estimated: 0, previous: 0))
    }

    @Test func longMonthSeriesAreShownByYear() throws {
        // 80 months: January 2020 to August 2026.
        let keys = (0..<80).map { String(format: "%04d-%02d", 2020 + $0 / 12, $0 % 12 + 1) }
        let series = ListeningSeries(
            try timeline(.month, keys: keys, values: Array(repeating: 1, count: 80)), measure: .plays)
        #expect(series.bucket == .year)
        #expect(series.columns.map(\.key) == ["2020", "2021", "2022", "2023", "2024", "2025", "2026"])
        #expect(series.columns.map(\.value) == [12, 12, 12, 12, 12, 12, 8])
        #expect(series.columns.map(\.id) == Array(0..<7))
    }

    @Test func shortMonthSeriesStayMonths() throws {
        let keys = (1...12).map { String(format: "2026-%02d", $0) }
        let series = ListeningSeries(
            try timeline(.month, keys: keys, values: Array(repeating: 1, count: 12)), measure: .plays)
        #expect(series.bucket == .month)
        #expect(series.columns.count == 12)
    }

    // MARK: - Labels

    @Test func labels() throws {
        let day = ListeningSeries(try timeline(.day, values: [1]), measure: .plays)
        #expect(day.label("2026-10-07", locale: us) == "Wed, Oct 7")
        #expect(day.tick("2026-10-07", locale: us) == "Oct 7")

        let week = ListeningSeries(try timeline(.week, values: [1]), measure: .plays)
        #expect(week.label("2026-10-12", locale: us) == "Oct 12, 2026")

        let month = ListeningSeries(try timeline(.month, keys: ["2026-10"], values: [1]), measure: .plays)
        #expect(month.label("2026-10", locale: us) == "Oct 2026")
        #expect(month.tick("2026-10", locale: us) == "Oct")
        #expect(month.tick("2027-01", locale: us) == "2027")

        let hour = ListeningSeries(try timeline(.hour, keys: ["2026-10-17T13"], values: [1]), measure: .plays)
        #expect(plain(hour.tick("2026-10-17T13", locale: us)) == "1 PM")
        #expect(plain(hour.label("2026-10-17T13", locale: us)) == "Oct 17 at 1 PM")
    }

    // MARK: - Helpers

    /// A timeline as the API sends it, from bucket keys and values.
    func timeline(
        _ bucket: ListeningSeries.Bucket, keys: [String]? = nil, values: [Int],
        estimated: [Int]? = nil, previous: [Int]? = nil
    ) throws -> Components.Schemas.Timeline {
        let keys = keys ?? values.indices.map { String(format: "2026-10-%02d", $0 + 1) }
        func points(_ values: [Int], keys: [String]) -> [[String: Any]] {
            values.indices.map { index in
                [
                    "key": keys[index], "start_at": 0, "plays": values[index],
                    "ms_played": values[index], "ms_estimated": estimated?[index] ?? 0,
                    "skips": 0, "artists": 0,
                ]
            }
        }
        let range: [String: Any] = [
            "preset": "last_4w", "from": "2026-10-01", "to": "2026-10-28", "start_at": 0,
            "end_at": 0, "timezone": "UTC", "first_play_date": "2021-01-01",
        ]
        var body: [String: Any] = [
            "bucket": bucket.rawValue, "range": range, "points": points(values, keys: keys),
            "previous": NSNull(),
        ]
        if let previous {
            body["previous"] = range
            body["previous_points"] = points(previous, keys: keys)
        }
        return try JSONDecoder().decode(
            Components.Schemas.Timeline.self, from: JSONSerialization.data(withJSONObject: body))
    }
}
