import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The longest sessions in the range on one panel, longest first, as the
/// web's Sessions: when, how long, how many plays, a meter against the
/// longest, and who was played most. Each opens its session. The next page
/// loads as the list nears its end.
struct SessionsList: View {
    let model: SessionsModel
    let range: StatsRange

    @Environment(Session.self) private var session
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private static let lookahead = 10

    var body: some View {
        Group {
            if model.hasFailed(for: range) {
                SectionFailure().panel()
            } else if let list = model.list {
                if list.sessions.isEmpty {
                    Text("No sessions in this range.")
                        .foregroundStyle(.secondary)
                        .panel()
                } else {
                    rows(list)
                }
            } else {
                placeholders
            }
        }
        .opacity(model.isStale(for: range) ? 0.55 : 1)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: model.isStale(for: range))
    }

    private func rows(_ list: SessionList) -> some View {
        let longest = Double(list.longest)
        let trigger = list.sessions[max(0, list.sessions.count - Self.lookahead)].id
        let now = Date.now
        return LazyVStack(spacing: 2) {
            ForEach(Array(list.sessions.enumerated()), id: \.element.id) { index, item in
                let share = Double(item.duration) / longest
                RouteLink(
                    route: .session(SessionLink(id: item.id, startedAt: item.startedAt)),
                    accessibilityLabel: SessionRow.spokenLabel(item, rank: index + 1, now: now),
                    accessibilityValue: SessionRow.spokenValue(item)
                ) {
                    SessionRow(rank: index + 1, session: item, share: share, now: now)
                }
                .onAppear {
                    if item.id == trigger { loadMore() }
                }
            }
            if !list.isComplete {
                PagingFooter(isFailed: model.moreFailed, loadMore: loadMore, retry: { model.retryMore(client: client) })
            }
        }
        .panel(padding: 12)
    }

    private func loadMore() {
        guard !model.isStale(for: range) else { return }
        model.loadMore(client: client)
    }

    private var placeholders: some View {
        VStack(spacing: 2) {
            ForEach(1...6, id: \.self) { rank in
                SessionRow.placeholder(rank: rank)
            }
        }
        .panel(padding: 12)
        .placeholder()
    }

    private var client: Client {
        session.client ?? .demo
    }
}
