import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The play log, newest first, a panel per day under a day heading that
/// stays at the top while its plays scroll by. Each play opens its track.
/// The next page loads as the log nears its end, until there is no more.
struct HistoryList: View {
    let model: HistoryModel
    let query: HistoryQuery
    /// Takes off the filters a person set.
    let clear: () -> Void

    @Environment(Session.self) private var session
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Namespace private var zoom

    /// How far from the end the next page is asked for.
    private static let lookahead = 20

    var body: some View {
        Group {
            if model.hasFailed(for: query) {
                SectionFailure().panel()
            } else if let log = model.log {
                if log.isEmpty {
                    empty
                } else {
                    days(log)
                }
            } else {
                placeholders
            }
        }
        .opacity(model.isStale(for: query) ? 0.55 : 1)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: model.isStale(for: query))
    }

    private func days(_ log: HistoryLog) -> some View {
        let trigger = log.play(fromEnd: Self.lookahead)?.id
        let now = Date.now
        return LazyVStack(alignment: .leading, spacing: 12, pinnedViews: .sectionHeaders) {
            ForEach(log.days) { day in
                Section {
                    VStack(spacing: 2) {
                        ForEach(day.plays) { play in
                            row(play, now: now)
                                .onAppear {
                                    if play.id == trigger { loadMore() }
                                }
                        }
                    }
                    .panel(padding: 12)
                } header: {
                    DayHeading(date: day.date, now: now)
                }
            }
            if !log.isComplete {
                PagingFooter(
                    isFailed: model.moreFailed, loadMore: loadMore, retry: { model.retryMore(client: client) })
            }
        }
    }

    private func row(_ play: RecentPlay, now: Date) -> some View {
        let zoom = ZoomOrigin(id: "history:\(play.id)", namespace: zoom)
        return RouteLink(
            route: .entity(EntityLink(kind: .track, id: play.trackID, name: play.name), zoom: zoom),
            accessibilityLabel: PlayRow.spokenLabel(play),
            accessibilityValue: PlayRow.spokenValue(play, now: now, style: .clock)
        ) {
            PlayRow(play: play, now: now, style: .clock, zoom: zoom)
        }
    }

    /// Only the log on screen pages; a dimmed one waits for its successor.
    private func loadMore() {
        guard !model.isStale(for: query) else { return }
        model.loadMore(client: client)
    }

    @ViewBuilder private var empty: some View {
        if query.filter.activeCount > 0 {
            VStack(alignment: .leading, spacing: 12) {
                Text("No plays match.")
                    .foregroundStyle(.secondary)
                Button("Clear filters", action: clear)
                    .buttonStyle(.bordered)
                    .tint(.primary)
            }
            .panel()
        } else {
            Text("No plays in this range.")
                .foregroundStyle(.secondary)
                .panel()
        }
    }

    private var placeholders: some View {
        VStack(alignment: .leading, spacing: 12) {
            DayHeading(date: .now, now: .now)
            VStack(spacing: 2) {
                ForEach(0..<8, id: \.self) { index in
                    PlayRow(
                        play: RecentPlay(
                            id: "placeholder-\(index)", trackID: "", name: "Placeholder track",
                            artists: "Placeholder artist", imageURL: nil, playedAt: .now, note: nil),
                        now: .now, style: .clock)
                }
            }
            .panel(padding: 12)
        }
        .redacted(reason: .placeholder)
        .accessibilityHidden(true)
    }

    private var client: Client {
        session.client ?? .demo
    }
}

/// "Today", "Yesterday", "Wed, Oct 21": on the page's ground, so plays
/// pass under it as it stays pinned.
private struct DayHeading: View {
    let date: Date
    let now: Date

    var body: some View {
        Text(Figures.day(date, now: now))
            .font(.footnote.weight(.semibold))
            .textCase(.uppercase)
            .foregroundStyle(.secondary)
            .padding(.horizontal, 4)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.ground)
            .accessibilityAddTraits(.isHeader)
    }
}
