import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// A list the API pages by offset, for one query at a time, with no cap:
/// the Library's songs, albums and playlists and a playlist's songs. The
/// last list stays on screen (dimmed) while a new query loads its first
/// page; the next page loads as the list nears its end.
@Observable
final class PagedListModel<Item: Identifiable & Hashable & Sendable, Query: Hashable & Sendable>
where Item.ID: Sendable {
    typealias Fetch = @Sendable (_ query: Query, _ offset: Int, _ limit: Int, _ client: Client) async throws ->
        OffsetPage<Item>

    private(set) var list: OffsetList<Item>?
    /// The query `list` answers.
    private(set) var answered: Query?
    private var failedQuery: Query?
    /// The last next page failed; a retry row asks again.
    private(set) var moreFailed = false

    @ObservationIgnored private let fetch: Fetch
    @ObservationIgnored private let pageSize: Int
    @ObservationIgnored private var moreTask: Task<Void, Never>?
    /// Bumped when a new first page replaces the list, so a next page still
    /// on its way for the old one is dropped.
    @ObservationIgnored private var generation = 0

    init(pageSize: Int = 50, fetch: @escaping Fetch) {
        self.pageSize = pageSize
        self.fetch = fetch
    }

    func isStale(for query: Query) -> Bool {
        list != nil && answered != query
    }

    /// The first page for `query` failed and there is nothing current to show.
    func hasFailed(for query: Query) -> Bool {
        failedQuery == query && answered != query
    }

    /// Loads the first page for `query`, unless it is on screen already.
    /// `force` reloads it anyway, for pull-to-refresh.
    func load(_ query: Query, client: Client, force: Bool = false) async {
        guard force || answered != query || list == nil else { return }
        do {
            let page = try await fetch(query, 0, pageSize, client)
            try Task.checkCancellation()
            start(page, for: query)
        } catch {
            // Cancelled: the screen went away or the query moved on.
            guard !Task.isCancelled else { return }
            failedQuery = query
        }
    }

    /// Starts the list with a first page loaded elsewhere (a playlist's
    /// songs come with the playlist).
    func start(_ page: OffsetPage<Item>, for query: Query) {
        moreTask?.cancel()
        moreTask = nil
        generation += 1
        moreFailed = false
        list = OffsetList(first: page)
        answered = query
        failedQuery = nil
    }

    /// Marks the first page for `query` as failed when it was loaded
    /// elsewhere.
    func fail(_ query: Query) {
        failedQuery = query
    }

    /// Asks for the next page, once, unless the list is complete or the
    /// last attempt failed.
    func loadMore(client: Client) {
        guard let list, let query = answered, !list.isComplete, moreTask == nil, !moreFailed else { return }
        let offset = list.nextOffset
        let generation = generation
        moreTask = Task { [weak self, fetch, pageSize] in
            let page = try? await fetch(query, offset, pageSize, client)
            self?.finishMore(page, at: offset, generation: generation)
        }
    }

    func retryMore(client: Client) {
        moreFailed = false
        loadMore(client: client)
    }

    private func finishMore(_ page: OffsetPage<Item>?, at offset: Int, generation: Int) {
        guard generation == self.generation else { return }
        moreTask = nil
        guard let page else {
            moreFailed = true
            return
        }
        list?.append(page, requestedAt: offset)
    }
}
