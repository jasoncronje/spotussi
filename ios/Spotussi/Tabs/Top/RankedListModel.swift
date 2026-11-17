import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// One kind's whole ranking for the range and measure, a page at a time
/// with no cap: the Top tab's lists and an entity's "All" lists. The last
/// list stays on screen (dimmed) while a new range loads its first page.
@Observable
final class RankedListModel {
    let kind: EntityKind
    /// The entity the ranking is narrowed to, if any.
    let scope: EntityLink?

    private(set) var list: RankedList?
    /// The query `list` answers.
    private(set) var answered: StatsQuery?
    private var failedQuery: StatsQuery?
    private(set) var isLoadingMore = false
    /// The last next page failed; a retry row asks again.
    private(set) var moreFailed = false

    @ObservationIgnored private var moreTask: Task<Void, Never>?
    /// Bumped when a new first page replaces the list, so a next page still
    /// on its way for the old one is dropped.
    @ObservationIgnored private var generation = 0

    static let pageSize = 50

    init(kind: EntityKind, within scope: EntityLink? = nil) {
        self.kind = kind
        self.scope = scope
    }

    func isStale(for query: StatsQuery) -> Bool {
        list != nil && answered != query
    }

    /// The first page for `query` failed and there is nothing current to show.
    func hasFailed(for query: StatsQuery) -> Bool {
        failedQuery == query && answered != query
    }

    /// Loads the first page for `query`, unless it is on screen already.
    /// `force` reloads it anyway, for pull-to-refresh.
    func load(_ query: StatsQuery, client: Client, force: Bool = false) async {
        guard force || answered != query || list == nil else { return }
        do {
            let page = try await TopPage.load(
                kind, within: scope, range: query.range, measure: query.measure, limit: Self.pageSize,
                client: client)
            try Task.checkCancellation()
            moreTask?.cancel()
            moreTask = nil
            generation += 1
            isLoadingMore = false
            moreFailed = false
            list = RankedList(first: page)
            answered = query
            failedQuery = nil
        } catch {
            // Cancelled: the screen went away or the range moved on.
            guard !Task.isCancelled else { return }
            failedQuery = query
        }
    }

    /// Asks for the next page, once, unless the list is complete or the
    /// last attempt failed.
    func loadMore(client: Client) {
        guard let list, let query = answered, !list.isComplete, moreTask == nil, !moreFailed else { return }
        let offset = list.nextOffset
        let generation = generation
        isLoadingMore = true
        moreTask = Task { [weak self, kind, scope] in
            let page = try? await TopPage.load(
                kind, within: scope, range: query.range, measure: query.measure, offset: offset,
                limit: Self.pageSize, client: client)
            self?.finishMore(page, at: offset, generation: generation)
        }
    }

    func retryMore(client: Client) {
        moreFailed = false
        loadMore(client: client)
    }

    private func finishMore(_ page: TopPage?, at offset: Int, generation: Int) {
        guard generation == self.generation else { return }
        moreTask = nil
        isLoadingMore = false
        guard let page else {
            moreFailed = true
            return
        }
        list?.append(page, requestedAt: offset)
    }
}
