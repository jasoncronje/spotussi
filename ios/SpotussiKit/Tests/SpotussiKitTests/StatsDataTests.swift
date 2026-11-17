import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

/// The Stats page's charts and the listening clock, reduced from the demo
/// data the way the web's Stats components read them.
@Suite struct StatsDataTests {
    let us = Locale(identifier: "en_US")
    let client = Client.demo

    // MARK: - Listening clock

    @Test func theClockHasEveryHour() async throws {
        let hours = HourlyListening(try await client.getClock().ok.body.json, measure: .plays)
        #expect(hours.hours.map(\.id) == Array(0..<24))
        #expect(hours.peak?.id == 14)
        #expect(hours.quietest?.id == 5)
        #expect(hours.hours.allSatisfy { $0.topArtist == nil && $0.topTrack == nil })
    }

    @Test func hourlyTopNamesEachHoursLeaders() async throws {
        let hours = HourlyListening(try await client.getHourlyTop().ok.body.json, measure: .plays)
        let ten = hours.hours[10]
        #expect(ten.plays == 46)
        #expect(ten.topArtist?.link == EntityLink(kind: .artist, id: "LL5tUHRKE86UEZLcvY0DYj", name: "Mirror Avalanche"))
        #expect(ten.topArtist?.value(for: .plays) == 7)
        #expect(ten.topTrack?.link.kind == .track)
        #expect(ten.topTrack?.detail == "Mirror Avalanche")
        let leader = try #require(hours.leadingArtist)
        #expect(leader.hours >= 1)
    }

    @Test func spokesLightToTheirShareOfTheBusiestHour() throws {
        let hours = HourlyListening(clock([0, 8, 4, 1] + Array(repeating: 0, count: 20)), measure: .plays)
        let busiest = hours.hours[1]
        #expect((0..<8).allSatisfy { hours.light(busiest, ring: $0) == 1 })
        let half = hours.hours[2]
        #expect(hours.light(half, ring: 3) == 1)
        #expect(hours.light(half, ring: 4) == 0)
        // A little listening still lights part of the first lamp.
        #expect(hours.light(hours.hours[3], ring: 0) == 1)
        #expect(hours.light(hours.hours[0], ring: 0) == 0)
        #expect(hours.share(busiest) == 8.0 / 13.0)
        #expect(hours.quietest?.id == 0)
    }

    @Test func aSilentDayHasNoPeak() {
        let hours = HourlyListening(clock(Array(repeating: 0, count: 24)), measure: .time)
        #expect(hours.isEmpty)
        #expect(hours.peak == nil)
        #expect(hours.quietest == nil)
        #expect(hours.share(hours.hours[0]) == 0)
    }

    @Test func hourLabels() {
        #expect(plain(Figures.hour(10, locale: us)) == "10 AM")
        #expect(plain(Figures.hour(0, locale: us)) == "12 AM")
        #expect(plain(Figures.hourSpan(22, locale: us)) == "10 – 11 PM")
    }

    // MARK: - Volume

    @Test func differentArtistsByDay() async throws {
        let timeline = try await client.getTimeline(query: .init(compare: true)).ok.body.json
        let series = ListeningSeries(timeline, quantity: .artists)
        #expect(series.columns.map(\.value) == timeline.points.map(\.artists))
        #expect(series.columns.allSatisfy { $0.estimated == 0 })
        #expect(series.measure == .plays)
        #expect(ListeningSeries(timeline, measure: .time).quantity == .time)
    }

    // MARK: - Shares over time

    @Test func artistSharesKeepFiveSeries() async throws {
        let breakdown = ShareBreakdown(try await client.getArtistsOverTime().ok.body.json, measure: .plays)
        #expect(breakdown.series.count == ShareBreakdown.maximumSeries)
        #expect(breakdown.series.first?.link.name == "Iris Sakoti")
        #expect(breakdown.series.allSatisfy { $0.link.kind == .artist })
        #expect(breakdown.columns.count == 28)
        #expect(breakdown.bucket == .day)
        #expect(breakdown.columns.allSatisfy { $0.shares.count == 5 && $0.shares.reduce(0, +) <= 1.000_001 })
        #expect(breakdown.held > 0 && breakdown.held < 1)
    }

    @Test func genresStackFromTheBottom() async throws {
        let breakdown = ShareBreakdown(try await client.getGenresOverTime().ok.body.json, measure: .time)
        #expect(breakdown.series.first?.link == EntityLink(kind: .genre, id: "modern-pop", name: "modern pop"))
        let column = breakdown.columns[3]
        // 2026-10-24: 44 plays, the first five genres 3, 6, 2, 1, 3.
        #expect(column.total == 44)
        let segments = breakdown.segments(column)
        #expect(segments.map(\.series) == [0, 1, 2, 3, 4])
        #expect(segments[0].start == 0)
        #expect(abs(segments[1].start - 3.0 / 44) < 1e-9)
        #expect(abs(segments[4].end - 15.0 / 44) < 1e-9)
        #expect(abs(column.other - 29.0 / 44) < 1e-9)
    }

    @Test func theScaleStopsJustAboveTheBusiestColumn() {
        let breakdown = ShareBreakdown(
            bucket: "day", links: [link("a"), link("b")],
            points: [point("2026-10-01", total: 100, values: [20, 11]), point("2026-10-02", total: 0, values: [0, 0])],
            measure: .plays)
        #expect(abs(breakdown.scale - 0.35) < 1e-9)
        #expect(breakdown.columns[1].other == 0)
        #expect(breakdown.segments(breakdown.columns[1]).isEmpty)
        #expect(abs(breakdown.held - 0.31) < 1e-9)
    }

    @Test func longMonthBreakdownsAreShownByYear() {
        let points = (0..<80).map { index in
            point(String(format: "%04d-%02d", 2020 + index / 12, index % 12 + 1), total: 10, values: [1])
        }
        let breakdown = ShareBreakdown(bucket: "month", links: [link("a")], points: points, measure: .plays)
        #expect(breakdown.bucket == .year)
        #expect(breakdown.columns.map(\.total) == [120, 120, 120, 120, 120, 120, 80])
        #expect(breakdown.columns.allSatisfy { abs($0.shares[0] - 0.1) < 1e-9 })
    }

    // MARK: - Averages

    @Test func releaseYears() async throws {
        let trend = AverageTrend(try await client.getReleaseYears().ok.body.json, kind: .year)
        #expect(trend.points.count == 28)
        #expect(trend.overall.map { trend.format($0, locale: us) } == "2019")
        let scale = trend.scale
        for value in trend.known.compactMap(\.value) {
            #expect(scale.domain.contains(value))
        }
        #expect(scale.ticks.allSatisfy { $0 == $0.rounded() })
        #expect(trend.change == (trend.known.last!.value! - trend.known.first!.value!).rounded())
    }

    @Test func artistsPerTrack() async throws {
        let trend = AverageTrend(try await client.getArtistsPerTrack().ok.body.json, kind: .ratio)
        #expect(trend.overall.map { trend.format($0, locale: us) } == "1.26")
        #expect(trend.known.allSatisfy { (0...1).contains($0.coverage) })
    }

    @Test func gapsBreakTheLine() throws {
        let trend = AverageTrend(try average([2001, nil, 2003, 2004, nil]), kind: .year)
        #expect(trend.runs.map { $0.map(\.id) } == [[0], [2, 3]])
        #expect(trend.known.count == 3)
        #expect(trend.change == 3)
    }

    @Test func nothingKnownHasNoOverall() throws {
        let trend = AverageTrend(try average([nil, nil]), kind: .ratio)
        #expect(trend.overall == nil)
        #expect(trend.change == nil)
        #expect(trend.scale.domain == 0...1)
    }

    // MARK: - Helpers

    func clock(_ plays: [Int]) -> Components.Schemas.Clock {
        Components.Schemas.Clock(
            hours: plays.enumerated().map { .init(hour: $0, msPlayed: $1 * 1000, plays: $1) },
            range: range)
    }

    func link(_ id: String) -> EntityLink {
        EntityLink(kind: .artist, id: id, name: id)
    }

    func point(_ key: String, total: Int, values: [Int]) -> Components.Schemas.BreakdownPoint {
        .init(key: key, other: max(0, total - values.reduce(0, +)), startAt: 0, total: total, values: values)
    }

    func average(_ values: [Double?]) throws -> Components.Schemas.AverageSeries {
        let known = values.compactMap(\.self)
        return Components.Schemas.AverageSeries(
            bucket: .day, metric: .plays,
            overall: known.isEmpty ? nil : known.reduce(0, +) / Double(known.count),
            points: values.enumerated().map { index, value in
                .init(coverage: value == nil ? 0 : 1, key: String(format: "2026-10-%02d", index + 1), startAt: 0, value: value)
            },
            range: range)
    }

    var range: Components.Schemas.ResolvedRange {
        try! JSONDecoder().decode(
            Components.Schemas.ResolvedRange.self,
            from: Data(
                #"{"preset":"last_4w","from":"2026-10-01","to":"2026-10-28","start_at":0,"end_at":0,"timezone":"UTC","first_play_date":"2021-01-01"}"#
                    .utf8))
    }
}
