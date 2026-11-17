import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The longest sessions in the shared range, longest first, a page at a
/// time with no cap. The last list stays on screen (dimmed) while a new
/// range loads its first page.
@Observable
final class SessionsModel {
    private(set) var list: SessionList?
    /// The range `list` answers.
    private(set) var answered: StatsRange?
    private var failedRange: StatsRange?
    private(set) var isLoadingMore = false
    private(set) var moreFailed = false

    @ObservationIgnored private var moreTask: Task<Void, Never>?
    @ObservationIgnored private var generation = 0

    func isStale(for range: StatsRange) -> Bool {
        list != nil && answered != range
    }

    func hasFailed(for range: StatsRange) -> Bool {
        failedRange == range && answered != range
    }

    func load(_ range: StatsRange, client: Client, force: Bool = false) async {
        guard force || answered != range || list == nil else { return }
        do {
            let page = try await SessionList.load(range: range, client: client)
            try Task.checkCancellation()
            moreTask?.cancel()
            moreTask = nil
            generation += 1
            isLoadingMore = false
            moreFailed = false
            list = SessionList(first: page)
            answered = range
            failedRange = nil
        } catch {
            guard !Task.isCancelled else { return }
            failedRange = range
        }
    }

    func loadMore(client: Client) {
        guard let list, let range = answered, !list.isComplete, moreTask == nil, !moreFailed else { return }
        let offset = list.nextOffset
        let generation = generation
        isLoadingMore = true
        moreTask = Task { [weak self] in
            let page = try? await SessionList.load(range: range, offset: offset, client: client)
            self?.finishMore(page, at: offset, generation: generation)
        }
    }

    func retryMore(client: Client) {
        moreFailed = false
        loadMore(client: client)
    }

    private func finishMore(_ page: Components.Schemas.Sessions?, at offset: Int, generation: Int) {
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
