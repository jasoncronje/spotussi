import SpotussiAPI
import SpotussiKit
import SwiftUI

/// A whole ranking on one panel, as the web's ranked table: rank, artwork,
/// name and artists, a meter against the first entry, the figure and its
/// movement. Rows are made as they scroll in; the next page loads as the
/// list nears its end, until the server has no more.
struct RankedListPanel: View {
    let model: RankedListModel
    let query: StatsQuery
    /// The screen's namespace, for zooming into a page from its row.
    let namespace: Namespace.ID

    @Environment(Session.self) private var session
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    /// How far from the end the next page is asked for: about two screens.
    private static let lookahead = 20

    var body: some View {
        Group {
            if model.hasFailed(for: query) {
                SectionFailure()
            } else if let list = model.list {
                if list.entries.isEmpty {
                    (model.kind == .genre ? Text("No genres yet.") : Text("No plays in this range."))
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

    private func rows(_ list: RankedList) -> some View {
        // The list keeps its own measure while a new one loads.
        let measure = model.answered?.measure ?? query.measure
        let leader = Double(list.leader(for: measure))
        let trigger = list.entries[max(0, list.entries.count - Self.lookahead)].id
        return LazyVStack(spacing: 2) {
            ForEach(list.entries) { entry in
                TopEntryLink(
                    entry: entry, measure: measure, share: Double(entry.value(for: measure)) / leader,
                    namespace: namespace
                )
                .onAppear {
                    if entry.id == trigger { model.loadMore(client: client) }
                }
            }
            if !list.isComplete {
                footer
            }
        }
    }

    /// Loading the next page, or one row to ask again when it failed.
    @ViewBuilder private var footer: some View {
        if model.moreFailed {
            Button {
                model.retryMore(client: client)
            } label: {
                HStack(spacing: 8) {
                    Text("Couldn’t load more.")
                        .foregroundStyle(.secondary)
                    Spacer(minLength: 8)
                    Label("Retry", systemImage: "arrow.clockwise")
                        .fontWeight(.medium)
                }
                .font(.subheadline)
                .frame(maxWidth: .infinity, minHeight: 44)
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, minHeight: 44)
                // Reaching the end without the trigger row having asked
                // (a short page, a fast fling) asks here.
                .onAppear { model.loadMore(client: client) }
                .accessibilityLabel("Loading more")
        }
    }

    private var placeholders: some View {
        VStack(spacing: 2) {
            ForEach(1...10, id: \.self) { rank in
                TopRow(entry: TopEntry.placeholder(kind: model.kind, rank: rank), measure: query.measure, share: 1 - Double(rank) / 12)
            }
        }
        .redacted(reason: .placeholder)
    }

    private var client: Client {
        session.client ?? .demo
    }
}
