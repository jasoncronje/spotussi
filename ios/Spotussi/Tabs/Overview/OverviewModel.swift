import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The Overview's sections for the shared range and its measure. Each
/// section loads on its own, all at once, like the web's Overview; what
/// loaded last stays on screen (dimmed) while the next range loads.
@Observable
final class OverviewModel {
    /// What the range-bound sections depend on; a change re-queries.
    nonisolated struct Query: Equatable, Sendable {
        let range: StatsRange
        let measure: Measure
    }

    private(set) var summary = Loaded<Components.Schemas.OverviewSummary>()
    private(set) var series = Loaded<ListeningSeries>()
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
        case top(TopEntry.Kind, [TopEntry]?)
    }

    private func apply(_ part: Part, for query: Query) {
        switch part {
        case .summary(let value): summary.finish(value, for: query)
        case .series(let value): series.finish(value, for: query)
        case .top(let kind, let value): tops[kind, default: Loaded()].finish(value, for: query)
        }
    }
}

/// A section's last answer and the query it answered, and whether the
/// latest query failed.
struct Loaded<Value> {
    private(set) var value: Value?
    private(set) var query: OverviewModel.Query?
    private var failedQuery: OverviewModel.Query?

    /// The value when it answers `query`.
    func current(for query: OverviewModel.Query) -> Value? {
        self.query == query ? value : nil
    }

    /// An answer for another range or measure, shown dimmed until the new
    /// one arrives.
    func isStale(for query: OverviewModel.Query) -> Bool {
        value != nil && self.query != query
    }

    /// The request for `query` failed and there is nothing current to show.
    func hasFailed(for query: OverviewModel.Query) -> Bool {
        failedQuery == query && self.query != query
    }

    mutating func finish(_ newValue: Value?, for query: OverviewModel.Query) {
        guard let newValue else {
            failedQuery = query
            return
        }
        value = newValue
        self.query = query
        failedQuery = nil
    }
}
