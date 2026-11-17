import SpotussiAPI
import SpotussiKit
import SwiftUI

/// How much was listened in the range, when, and to what, then the latest
/// plays. Re-queried when the range or measure changes; recent plays when
/// the playing track changes.
struct OverviewTab: View {
    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @Environment(NowPlayingModel.self) private var nowPlaying
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model = OverviewModel()

    var body: some View {
        TabPage(
            title: "Overview", subtitle: Text(subtitle), controls: .rangeAndMeasure(.overview),
            refresh: { [model, query, client] in
                async let range: Void = model.load(query, client: client)
                async let recent: Void = model.loadRecent(client: client)
                _ = await (range, recent)
            }
        ) {
            if model.isEmpty(for: query) {
                Text("No plays in this range.")
                    .foregroundStyle(.secondary)
                    .panel()
            } else {
                rangeSections
            }
            RecentPlaysPanel(plays: model.recent, failed: model.recentFailed)
        }
        .task(id: query) { await model.load(query, client: client) }
        .task(id: nowPlaying.track?.id) { await model.loadRecent(client: client) }
        .animation(reduceMotion ? nil : .smooth, value: model.isEmpty(for: query))
    }

    @ViewBuilder private var rangeSections: some View {
        let measure = query.measure
        section(model.summary) { summary, isFailed in
            if isFailed {
                SectionFailure().panel()
            } else {
                SummaryGrid(summary: summary, measure: measure, compareLabel: options.range.compareLabel)
            }
        }
        section(model.series) { series, isFailed in
            PlaysByDayPanel(series: series, isFailed: isFailed)
        }
        ForEach(TopEntry.Kind.allCases, id: \.self) { kind in
            section(model.top(kind)) { entries, isFailed in
                TopFivePanel(kind: kind, entries: entries, measure: measure, isFailed: isFailed)
            }
        }
    }

    /// A section with its latest answer: placeholders until the first one,
    /// dimmed while the next range loads, one quiet line if it failed.
    private func section<Value, Content: View>(
        _ loaded: Loaded<Value>, @ViewBuilder content: (Value?, Bool) -> Content
    ) -> some View {
        let isFailed = loaded.hasFailed(for: query)
        let isStale = loaded.isStale(for: query) && !isFailed
        return content(isFailed ? nil : loaded.value, isFailed)
            .redacted(reason: loaded.value == nil && !isFailed ? .placeholder : [])
            .opacity(isStale ? 0.55 : 1)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: isStale)
    }

    private var query: OverviewModel.Query {
        OverviewModel.Query(range: options.range, measure: options.measure(for: .overview))
    }

    private var client: Client {
        session.client ?? .demo
    }

    private var subtitle: String {
        guard let range = model.summary.value?.range,
            let dates = StatsRange.interval(range.from, range.to)
        else { return options.range.longLabel }
        return dates
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { OverviewTab() }
}

#Preview("Largest text") {
    PreviewHost(.demo) { OverviewTab() }
        .dynamicTypeSize(.accessibility5)
}

#Preview("Light") {
    PreviewHost(.demo) { OverviewTab() }
        .preferredColorScheme(.light)
}
