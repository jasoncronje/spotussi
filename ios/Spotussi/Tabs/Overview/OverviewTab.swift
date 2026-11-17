import SpotussiAPI
import SpotussiKit
import SwiftUI

/// For now: the summary figures for the shared range, re-queried when the
/// range or measure changes. I2 replaces the body with the full Overview.
struct OverviewTab: View {
    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model = OverviewModel()

    var body: some View {
        TabPage(
            title: "Overview", subtitle: Text(subtitle), controls: .rangeAndMeasure(.overview),
            refresh: { [model, query, client] in await model.load(query, client: client) }
        ) {
            if let summary = model.summary {
                figures(summary)
                topPanel(summary)
            } else if model.didFail {
                Text("The summary didn’t load. Pull down to try again.")
                    .foregroundStyle(.secondary)
                    .panel()
            } else {
                figures(nil)
                    .redacted(reason: .placeholder)
            }
            ComingNextPanel(
                symbol: "chart.dots.scatter",
                title: "Plays by day, top fives and recent plays",
                message: "The rest of the Overview arrives in the next update.")
        }
        .task(id: query) { await model.load(query, client: client) }
        .animation(reduceMotion ? nil : .smooth, value: model.summary)
    }

    private var query: OverviewModel.Query {
        OverviewModel.Query(range: options.range, measure: options.measure(for: .overview))
    }

    private var client: Client {
        session.client ?? .demo
    }

    private var subtitle: String {
        guard let range = model.summary?.range,
            let dates = StatsRange.interval(range.from, range.to)
        else { return options.range.longLabel }
        return dates
    }

    private var columns: [GridItem] {
        let count = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        return Array(repeating: GridItem(.flexible(), spacing: 12), count: count)
    }

    // MARK: - Panels

    private func figures(_ summary: Components.Schemas.OverviewSummary?) -> some View {
        let current = summary?.current
        let previous = summary?.previousSummary
        return VStack(alignment: .leading, spacing: 10) {
            if let compare = options.range.compareLabel, summary?.previousSummary != nil {
                Text("Changes are \(compare).")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            LazyVGrid(columns: columns, spacing: 12) {
                Readout(
                    label: "Time listened",
                    value: Self.hours(current?.msPlayed ?? 0),
                    change: Self.change(current?.msPlayed, previous?.msPlayed))
                Readout(
                    label: "Plays",
                    value: (current?.plays ?? 0).formatted(),
                    change: Self.change(current?.plays, previous?.plays))
                Readout(
                    label: "Tracks",
                    value: (current?.tracks ?? 0).formatted(),
                    change: Self.change(current?.tracks, previous?.tracks))
                Readout(
                    label: "Artists",
                    value: (current?.artists ?? 0).formatted(),
                    change: Self.change(current?.artists, previous?.artists))
            }
        }
    }

    @ViewBuilder private func topPanel(_ summary: Components.Schemas.OverviewSummary) -> some View {
        let measure = options.measure(for: .overview)
        if summary.current.topArtist != nil || summary.current.topTrack != nil {
            VStack(alignment: .leading, spacing: 14) {
                (measure == .plays ? Text("Most played") : Text("Most listened to"))
                    .font(.readoutLabel)
                    .foregroundStyle(.secondary)
                if let artist = summary.current.topArtist {
                    TopLine(
                        kind: "Artist", name: artist.artist.name,
                        figure: Self.figure(plays: artist.plays, ms: artist.msPlayed, measure: measure))
                }
                if let track = summary.current.topTrack {
                    TopLine(
                        kind: "Track", name: track.track.name,
                        figure: Self.figure(plays: track.plays, ms: track.msPlayed, measure: measure))
                }
            }
            .panel()
        }
    }

    // MARK: - Formatting

    static func hours(_ milliseconds: Int) -> String {
        Duration.milliseconds(milliseconds)
            .formatted(.units(allowed: [.hours, .minutes], width: .narrow, maximumUnitCount: 1))
    }

    static func figure(plays: Int, ms: Int, measure: Measure) -> String {
        switch measure {
        case .plays: String(localized: "\(plays) plays")
        case .time:
            Duration.milliseconds(ms)
                .formatted(.units(allowed: [.hours, .minutes], width: .abbreviated, maximumUnitCount: 2))
        }
    }

    /// Whole percent change, or nil without a previous period to compare.
    static func change(_ current: Int?, _ previous: Int?) -> Int? {
        guard let current, let previous, previous > 0 else { return nil }
        return Int((Double(current - previous) / Double(previous) * 100).rounded())
    }
}

/// "Artist · Alho · 29 plays", wrapping rather than truncating.
private struct TopLine: View {
    let kind: LocalizedStringKey
    let name: String
    let figure: String

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline) {
                labelAndName
                Spacer(minLength: 12)
                figureText
            }
            VStack(alignment: .leading, spacing: 2) {
                labelAndName
                figureText
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var labelAndName: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(kind)
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(name)
                .font(.body.weight(.semibold))
        }
    }

    private var figureText: some View {
        Text(figure)
            .font(.subheadline.monospacedDigit())
            .foregroundStyle(.secondary)
    }
}

#Preview {
    PreviewHost(.demo) { OverviewTab() }
}
