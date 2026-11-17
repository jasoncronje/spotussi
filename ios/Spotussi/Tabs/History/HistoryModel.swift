import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// What the play log depends on; a change loads it again.
nonisolated struct HistoryQuery: Hashable, Sendable {
    let range: StatsRange
    let filter: HistoryFilter
}

/// The play log for a range and its filters, newest first, a page at a
/// time by cursor until the server has no more, as the web's History. The
/// last log stays on screen (dimmed) while new filters or a new range load
/// their first page. Also the devices the log can be narrowed to.
@Observable
final class HistoryModel {
    private(set) var log: HistoryLog?
    /// The query `log` answers.
    private(set) var answered: HistoryQuery?
    private var failedQuery: HistoryQuery?
    private(set) var isLoadingMore = false
    /// The last next page failed; a retry row asks again.
    private(set) var moreFailed = false
    /// Devices and platforms seen in the range, for the filter menu.
    private(set) var devices: PlaybackDevices?

    @ObservationIgnored private var moreTask: Task<Void, Never>?
    /// Bumped when a new first page replaces the log, so a next page still
    /// on its way for the old one is dropped.
    @ObservationIgnored private var generation = 0

    func isStale(for query: HistoryQuery) -> Bool {
        log != nil && answered != query
    }

    /// The first page for `query` failed and there is nothing current to show.
    func hasFailed(for query: HistoryQuery) -> Bool {
        failedQuery == query && answered != query
    }

    /// Loads the first page for `query`, unless it is on screen already.
    /// `force` reloads it anyway, for pull-to-refresh.
    func load(_ query: HistoryQuery, client: Client, force: Bool = false) async {
        guard force || answered != query || log == nil else { return }
        do {
            let page = try await HistoryLog.load(range: query.range, filter: query.filter, client: client)
            try Task.checkCancellation()
            moreTask?.cancel()
            moreTask = nil
            generation += 1
            isLoadingMore = false
            moreFailed = false
            log = HistoryLog(first: page)
            answered = query
            failedQuery = nil
        } catch {
            // Cancelled: the screen went away or the query moved on.
            guard !Task.isCancelled else { return }
            failedQuery = query
        }
    }

    /// The devices for the filter menu; without them the menu offers none.
    func loadDevices(_ range: StatsRange, client: Client) async {
        let span = range.query
        guard
            let answer = try? await client.getDevices(query: .init(range: span.range, from: span.from, to: span.to))
                .ok.body.json
        else { return }
        devices = PlaybackDevices(answer)
    }

    /// Asks for the next page, once, unless the log is complete or the
    /// last attempt failed.
    func loadMore(client: Client) {
        guard let log, let query = answered, let cursor = log.nextCursor, !log.isComplete, moreTask == nil,
            !moreFailed
        else { return }
        let generation = generation
        isLoadingMore = true
        moreTask = Task { [weak self] in
            let page = try? await HistoryLog.load(
                range: query.range, filter: query.filter, cursor: cursor, client: client)
            self?.finishMore(page, generation: generation)
        }
    }

    func retryMore(client: Client) {
        moreFailed = false
        loadMore(client: client)
    }

    private func finishMore(_ page: Components.Schemas.History?, generation: Int) {
        guard generation == self.generation else { return }
        moreTask = nil
        isLoadingMore = false
        guard let page else {
            moreFailed = true
            return
        }
        log?.append(page)
    }
}
