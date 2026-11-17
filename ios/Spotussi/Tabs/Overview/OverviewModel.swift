import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The Overview's sections for the shared range and its measure: the
/// summary, listening by day and around the clock, and the top fives. Each
/// section loads on its own, all at once, like the web's Overview; what
/// loaded last stays on screen (dimmed) while the next range loads.
@Observable
final class OverviewModel {
    typealias Query = StatsQuery

    private(set) var summary = Loaded<Components.Schemas.OverviewSummary>()
    private(set) var series = Loaded<ListeningSeries>()
    private(set) var clock = Loaded<HourlyListening>()
    private var tops: [TopEntry.Kind: Loaded<[TopEntry]>] = [:]

    /// The latest plays, whatever the range, as the web's recent strip.
    private(set) var recent: [RecentPlay]?
    private(set) var recentFailed = false

    func top(_ kind: TopEntry.Kind) -> Loaded<[TopEntry]> {
        tops[kind] ?? Loaded()
    }

    /// The range had nothing in it.
    func isEmpty(for query: Query) -> Bool {
        guard let current = summary.current(for: query)?.current else { return false }
        return current.plays == 0 && current.msPlayed == 0
    }

    // MARK: - Loading

    /// Loads every range-bound section at once and shows each as it lands.
    /// Leaving the screen or changing the range again cancels the rest.
    func load(_ query: Query, client: Client) async {
        let range = query.range.query
        let metric = query.measure.metric
        await withTaskGroup(of: Part.self) { group in
            group.addTask {
                .summary(
                    try? await client.getOverviewSummary(
                        query: .init(range: range.range, from: range.from, to: range.to, metric: metric)
                    ).ok.body.json)
            }
            group.addTask {
                let timeline = try? await client.getTimeline(
                    query: .init(range: range.range, from: range.from, to: range.to, compare: true)
                ).ok.body.json
                return .series(timeline.map { ListeningSeries($0, measure: query.measure) })
            }
            group.addTask {
                let clock = try? await client.getClock(
                    query: .init(range: range.range, from: range.from, to: range.to)
                ).ok.body.json
                return .clock(clock.map { HourlyListening($0, measure: query.measure) })
            }
            group.addTask {
                let top = try? await client.getTopArtists(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, limit: 5)
                ).ok.body.json
                return .top(.artist, top.map { TopEntry.list($0) })
            }
            group.addTask {
                let top = try? await client.getTopTracks(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, limit: 5)
                ).ok.body.json
                return .top(.track, top.map { TopEntry.list($0) })
            }
            group.addTask {
                let top = try? await client.getTopAlbums(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, limit: 5)
                ).ok.body.json
                return .top(.album, top.map { TopEntry.list($0) })
            }
            group.addTask {
                let top = try? await client.getTopGenres(
                    query: .init(range: range.range, from: range.from, to: range.to, metric: metric, limit: 5)
                ).ok.body.json
                return .top(.genre, top.map { TopEntry.list($0) })
            }
            for await part in group {
                // A cancelled request isn't a failure: the screen went away
                // or the range moved on, and the next load fills in.
                guard !Task.isCancelled else { return }
                apply(part, for: query)
            }
        }
    }

    func loadRecent(client: Client) async {
        do {
            let history = try await client.getHistory(query: .init(limit: 10)).ok.body.json
            recent = RecentPlay.list(history)
            recentFailed = false
        } catch {
            guard !Task.isCancelled else { return }
            recentFailed = true
        }
    }

    /// One section's answer; nil when its request failed.
    private nonisolated enum Part: Sendable {
        case summary(Components.Schemas.OverviewSummary?)
        case series(ListeningSeries?)
        case clock(HourlyListening?)
        case top(TopEntry.Kind, [TopEntry]?)
    }

    private func apply(_ part: Part, for query: Query) {
        switch part {
        case .summary(let value): summary.finish(value, for: query)
        case .series(let value): series.finish(value, for: query)
        case .clock(let value): clock.finish(value, for: query)
        case .top(let kind, let value): tops[kind, default: Loaded()].finish(value, for: query)
        }
    }
}
