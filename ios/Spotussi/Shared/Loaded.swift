import SpotussiKit
import SwiftUI

/// What a range-bound section depends on; a change re-queries.
nonisolated struct StatsQuery: Hashable, Sendable {
    let range: StatsRange
    let measure: Measure
}

/// A section's last answer and the query it answered, and whether the
/// latest query failed.
struct Loaded<Value> {
    private(set) var value: Value?
    private(set) var query: StatsQuery?
    private var failedQuery: StatsQuery?

    /// The value when it answers `query`.
    func current(for query: StatsQuery) -> Value? {
        self.query == query ? value : nil
    }

    /// An answer for another range or measure, shown dimmed until the new
    /// one arrives.
    func isStale(for query: StatsQuery) -> Bool {
        value != nil && self.query != query
    }

    /// The request for `query` failed and there is nothing current to show.
    func hasFailed(for query: StatsQuery) -> Bool {
        failedQuery == query && self.query != query
    }

    mutating func finish(_ newValue: Value?, for query: StatsQuery) {
        guard let newValue else {
            failedQuery = query
            return
        }
        value = newValue
        self.query = query
        failedQuery = nil
    }
}

/// A section with its latest answer: placeholders until the first one,
/// dimmed while the next range loads, one quiet line if it failed.
struct LoadedSection<Value, Content: View>: View {
    let loaded: Loaded<Value>
    let query: StatsQuery
    /// The value (nil before the first answer or after a failure) and
    /// whether the request failed.
    @ViewBuilder let content: (Value?, Bool) -> Content
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    init(_ loaded: Loaded<Value>, for query: StatsQuery, @ViewBuilder content: @escaping (Value?, Bool) -> Content) {
        self.loaded = loaded
        self.query = query
        self.content = content
    }

    var body: some View {
        let isFailed = loaded.hasFailed(for: query)
        let isStale = loaded.isStale(for: query) && !isFailed
        content(isFailed ? nil : loaded.value, isFailed)
            .redacted(reason: loaded.value == nil && !isFailed ? .placeholder : [])
            .opacity(isStale ? 0.55 : 1)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: isStale)
    }
}
