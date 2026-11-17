import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The Stats page's charts for the shared range and its measure, as the
/// web's Stats page: listening over time, how much of it the top artists
/// hold, the listening clock with each hour's leaders, the average release
/// year and artists per track, and genres over time. Each loads on its own,
/// all at once, and keeps its last answer (dimmed) while a new range loads.
@Observable
final class StatsModel {
    /// Kept whole: the panel picks plays, time or artists from it.
    private(set) var timeline = Loaded<Components.Schemas.Timeline>()
    private(set) var artists = Loaded<ShareBreakdown>()
    private(set) var hours = Loaded<HourlyListening>()
    private(set) var releaseYears = Loaded<AverageTrend>()
    private(set) var artistsPerTrack = Loaded<AverageTrend>()
    private(set) var genres = Loaded<ShareBreakdown>()

    /// The range had nothing in it.
    func isEmpty(for query: StatsQuery) -> Bool {
        guard let timeline = timeline.current(for: query) else { return false }
        return timeline.points.allSatisfy { $0.plays == 0 && $0.msPlayed == 0 }
    }

    func load(_ query: StatsQuery, client: Client) async {
        let range = query.range.query
        let metric = query.measure.metric
        let measure = query.measure
        let series = ShareBreakdown.maximumSeries
        await withTaskGroup(of: Part.self) { group in
            group.addTask {
                .timeline(
                    try? await client.getTimeline(
                        query: .init(range: range.range, from: range.from, to: range.to, compare: true)
                    ).ok.body.json)
            }
            group.addTask {
                let answer = try? await client.getArtistsOverTime(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, series: series)
                ).ok.body.json
                return .artists(answer.map { ShareBreakdown($0, measure: measure) })
            }
            group.addTask {
                let answer = try? await client.getHourlyTop(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric)
                ).ok.body.json
                return .hours(answer.map { HourlyListening($0, measure: measure) })
            }
            group.addTask {
                let answer = try? await client.getReleaseYears(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric)
                ).ok.body.json
                return .releaseYears(answer.map { AverageTrend($0, kind: .year) })
            }
            group.addTask {
                let answer = try? await client.getArtistsPerTrack(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric)
                ).ok.body.json
                return .artistsPerTrack(answer.map { AverageTrend($0, kind: .ratio) })
            }
            group.addTask {
                let answer = try? await client.getGenresOverTime(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, series: series)
                ).ok.body.json
                return .genres(answer.map { ShareBreakdown($0, measure: measure) })
            }
            for await part in group {
                // A cancelled request isn't a failure: the screen went away
                // or the range moved on, and the next load fills in.
                guard !Task.isCancelled else { return }
                apply(part, for: query)
            }
        }
    }

    /// One chart's answer; nil when its request failed.
    private nonisolated enum Part: Sendable {
        case timeline(Components.Schemas.Timeline?)
        case artists(ShareBreakdown?)
        case hours(HourlyListening?)
        case releaseYears(AverageTrend?)
        case artistsPerTrack(AverageTrend?)
        case genres(ShareBreakdown?)
    }

    private func apply(_ part: Part, for query: StatsQuery) {
        switch part {
        case .timeline(let value): timeline.finish(value, for: query)
        case .artists(let value): artists.finish(value, for: query)
        case .hours(let value): hours.finish(value, for: query)
        case .releaseYears(let value): releaseYears.finish(value, for: query)
        case .artistsPerTrack(let value): artistsPerTrack.finish(value, for: query)
        case .genres(let value): genres.finish(value, for: query)
        }
    }
}
