import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// An artist, album, track or genre page for the shared range and the
/// page's measure: its head and totals, listening over time and around the
/// clock, what inside
/// it was played most, and where it was played from. Like the Overview,
/// each section loads on its own, all at once, and keeps its last answer
/// (dimmed) while a new range loads.
@Observable
final class EntityModel {
    let link: EntityLink

    private(set) var detail = Loaded<EntityDetail>()
    /// The instance has no such entity.
    private(set) var isNotFound = false
    private(set) var series = Loaded<ListeningSeries>()
    private(set) var clock = Loaded<HourlyListening>()
    private var lists: [EntityKind: Loaded<TopPage>] = [:]
    private(set) var contexts = Loaded<PlayContexts>()
    /// A track's latest plays in the range.
    private(set) var plays = Loaded<[RecentPlay]>()

    init(link: EntityLink) {
        self.link = link
    }

    /// A ranking inside the entity and how many rows it shows.
    nonisolated struct Listed: Hashable, Sendable {
        let kind: EntityKind
        let count: Int
    }

    /// What each page ranks inside its entity, in order, as the web's
    /// entity view: a track has nothing inside it to rank.
    var listed: [Listed] {
        switch link.kind {
        case .artist: [Listed(kind: .track, count: 8), Listed(kind: .album, count: 5)]
        case .album: [Listed(kind: .track, count: 12), Listed(kind: .artist, count: 5)]
        case .track: []
        case .genre: [Listed(kind: .artist, count: 5), Listed(kind: .track, count: 5), Listed(kind: .album, count: 5)]
        }
    }

    func list(_ kind: EntityKind) -> Loaded<TopPage> {
        lists[kind] ?? Loaded()
    }

    // MARK: - Loading

    func load(_ query: StatsQuery, client: Client) async {
        let link = link
        let listed = listed
        let range = query.range.query
        let metric = query.measure.metric
        let scope = Scope(link)
        await withTaskGroup(of: Part.self) { group in
            group.addTask {
                do {
                    return .detail(
                        try await EntityDetail.load(
                            link.kind, id: link.id, range: query.range, measure: query.measure, client: client),
                        notFound: false)
                } catch {
                    return .detail(nil, notFound: (error as? EntityLoadError) == .notFound)
                }
            }
            group.addTask {
                let timeline = try? await client.getTimeline(
                    query: .init(
                        range: range.range, from: range.from, to: range.to, artistId: scope.artist,
                        albumId: scope.album, trackId: scope.track, genreId: scope.genre, compare: true)
                ).ok.body.json
                return .series(timeline.map { ListeningSeries($0, measure: query.measure) })
            }
            group.addTask {
                let clock = try? await client.getClock(
                    query: .init(
                        range: range.range, from: range.from, to: range.to, artistId: scope.artist,
                        albumId: scope.album, trackId: scope.track, genreId: scope.genre)
                ).ok.body.json
                return .clock(clock.map { HourlyListening($0, measure: query.measure) })
            }
            for item in listed {
                group.addTask {
                    let page = try? await TopPage.load(
                        item.kind, within: link, range: query.range, measure: query.measure, limit: item.count,
                        client: client)
                    return .list(item.kind, page)
                }
            }
            group.addTask {
                let contexts = try? await client.getContexts(
                    query: .init(
                        range: range.range, from: range.from, to: range.to, metric: metric, limit: 8,
                        artistId: scope.artist, albumId: scope.album, trackId: scope.track, genreId: scope.genre)
                ).ok.body.json
                return .contexts(contexts.map(PlayContexts.init))
            }
            if link.kind == .track {
                group.addTask {
                    let history = try? await client.getHistory(
                        query: .init(range: range.range, from: range.from, to: range.to, trackId: link.id, limit: 10)
                    ).ok.body.json
                    return .plays(history.map { RecentPlay.list($0) })
                }
            }
            for await part in group {
                // A cancelled request isn't a failure: the page went away
                // or the range moved on, and the next load fills in.
                guard !Task.isCancelled else { return }
                apply(part, for: query)
            }
        }
    }

    /// One section's answer; nil when its request failed.
    private nonisolated enum Part: Sendable {
        case detail(EntityDetail?, notFound: Bool)
        case series(ListeningSeries?)
        case clock(HourlyListening?)
        case list(EntityKind, TopPage?)
        case contexts(PlayContexts?)
        case plays([RecentPlay]?)
    }

    private func apply(_ part: Part, for query: StatsQuery) {
        switch part {
        case .detail(let value, let notFound):
            isNotFound = notFound
            detail.finish(value, for: query)
        case .series(let value): series.finish(value, for: query)
        case .clock(let value): clock.finish(value, for: query)
        case .list(let kind, let value): lists[kind, default: Loaded()].finish(value, for: query)
        case .contexts(let value): contexts.finish(value, for: query)
        case .plays(let value): plays.finish(value, for: query)
        }
    }

    /// The entity as the API's scope parameters: one of them set.
    private nonisolated struct Scope: Sendable {
        var artist: String?
        var album: String?
        var track: String?
        var genre: String?

        init(_ link: EntityLink) {
            switch link.kind {
            case .artist: artist = link.id
            case .album: album = link.id
            case .track: track = link.id
            case .genre: genre = link.id
            }
        }
    }
}
