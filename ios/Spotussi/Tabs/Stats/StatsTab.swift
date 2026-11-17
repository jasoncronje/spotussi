import SpotussiAPI
import SpotussiKit
import SwiftUI

/// How listening changes over time, in the web's order (D14): listening
/// by day, the top artists' share, the listening clock with each hour's
/// leaders, the average release year, artists per track, and genres.
/// Re-queried when the range or this screen's measure changes.
struct StatsTab: View {
    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model = StatsModel()
    /// What listening over time counts; nil follows the measure, as the
    /// web's volume chart does until a choice is made there.
    @State private var volume: ListeningSeries.Quantity?

    var body: some View {
        let query = self.query
        TabPage(
            title: "Stats", subtitle: Text(subtitle), controls: .rangeAndMeasure(.stats),
            refresh: { [model, query, client] in
                await model.load(query, client: client)
            }
        ) {
            if model.isEmpty(for: query) {
                Text("No plays in this range.")
                    .foregroundStyle(.secondary)
                    .panel()
            } else {
                charts(query)
            }
        }
        .task(id: query) { await model.load(query, client: client) }
        .animation(reduceMotion ? nil : .smooth, value: model.isEmpty(for: query))
    }

    @ViewBuilder private func charts(_ query: StatsQuery) -> some View {
        let quantity = volume ?? ListeningSeries.Quantity(query.measure)
        LoadedSection(model.timeline, for: query) { timeline, isFailed in
            PlaysByDayPanel(
                series: timeline.map { ListeningSeries($0, quantity: quantity) }, isFailed: isFailed, screen: .stats,
                choice: Binding(get: { quantity }, set: { volume = $0 }))
        }
        LoadedSection(model.artists, for: query) { breakdown, isFailed in
            SharePanel(
                title: String(localized: "Artist share"), kind: .artist, breakdown: breakdown, isFailed: isFailed)
        }
        LoadedSection(model.hours, for: query) { hours, isFailed in
            ListeningClockPanel(hours: hours, isFailed: isFailed, measure: query.measure)
        }
        LoadedSection(model.releaseYears, for: query) { trend, isFailed in
            AveragePanel(
                title: String(localized: "Release year"), trend: trend, isFailed: isFailed,
                emptyText: "No release dates yet.")
        }
        LoadedSection(model.artistsPerTrack, for: query) { trend, isFailed in
            AveragePanel(
                title: String(localized: "Artists per track"), trend: trend, isFailed: isFailed,
                emptyText: "No plays in this range.")
        }
        LoadedSection(model.genres, for: query) { breakdown, isFailed in
            SharePanel(
                title: String(localized: "Genres over time"), kind: .genre, breakdown: breakdown, isFailed: isFailed)
        }
    }

    private var query: StatsQuery {
        StatsQuery(range: options.range, measure: options.measure(for: .stats))
    }

    private var client: Client {
        session.client ?? .demo
    }

    private var subtitle: String {
        guard let range = model.timeline.value?.range,
            let dates = StatsRange.interval(range.from, range.to)
        else { return options.range.longLabel }
        return dates
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { StatsTab() }
}

#Preview("Largest text") {
    PreviewHost(.demo) { StatsTab() }
        .dynamicTypeSize(.accessibility5)
}

#Preview("Light") {
    PreviewHost(.demo) { StatsTab() }
        .preferredColorScheme(.light)
}
