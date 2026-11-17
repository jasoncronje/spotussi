import SpotussiAPI
import SpotussiKit
import SwiftUI

/// A paged list on one panel: placeholder rows until the first page, one
/// quiet line when it failed or is empty, rows made as they scroll in, and
/// the next page asked for as the list nears its end.
struct PagedPanel<
    Item: Identifiable & Hashable & Sendable, Query: Hashable & Sendable, Row: View, Placeholder: View
>: View where Item.ID: Sendable {
    let model: PagedListModel<Item, Query>
    let query: Query
    let empty: LocalizedStringKey
    /// One row drawn redacted, several times, while the first page loads.
    let placeholder: Placeholder
    @ViewBuilder let row: (Item) -> Row

    init(
        model: PagedListModel<Item, Query>, query: Query, empty: LocalizedStringKey,
        @ViewBuilder placeholder: () -> Placeholder, @ViewBuilder row: @escaping (Item) -> Row
    ) {
        self.model = model
        self.query = query
        self.empty = empty
        self.placeholder = placeholder()
        self.row = row
    }

    @Environment(Session.self) private var session
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    /// How far from the end the next page is asked for: about two screens.
    private static var lookahead: Int { 20 }

    var body: some View {
        Group {
            if model.hasFailed(for: query) {
                SectionFailure()
            } else if let list = model.list {
                if list.isEmpty {
                    Text(empty)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                } else {
                    rows(list)
                }
            } else {
                placeholders
            }
        }
        .opacity(model.isStale(for: query) ? 0.55 : 1)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: model.isStale(for: query))
        .panel(padding: 12)
    }

    private func rows(_ list: OffsetList<Item>) -> some View {
        let trigger = list.items[max(0, list.items.count - Self.lookahead)].id
        return LazyVStack(spacing: 2) {
            ForEach(list.items) { item in
                row(item)
                    .onAppear {
                        if item.id == trigger { loadMore() }
                    }
            }
            if !list.isComplete {
                PagingFooter(isFailed: model.moreFailed, loadMore: loadMore, retry: { model.retryMore(client: client) })
            }
        }
    }

    /// Only the list on screen pages; a dimmed one waits for its successor.
    private func loadMore() {
        guard !model.isStale(for: query) else { return }
        model.loadMore(client: client)
    }

    private var placeholders: some View {
        VStack(spacing: 2) {
            ForEach(0..<8, id: \.self) { _ in
                placeholder
            }
        }
        .redacted(reason: .placeholder)
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private var client: Client {
        session.client ?? .demo
    }
}
