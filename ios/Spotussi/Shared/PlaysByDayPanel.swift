import SpotussiKit
import SwiftUI

/// Plays, time or different artists per bucket of the range on the lamp
/// grid, with the peak above it and dates below. VoiceOver reads a summary
/// and can play the series as an audio graph.
struct PlaysByDayPanel: View {
    let series: ListeningSeries?
    let isFailed: Bool
    /// Whose Plays/Time choice titles the panel before the series is in.
    let screen: ViewOptions.Screen
    /// What the grid counts, chosen on the panel (Stats); nil follows the
    /// screen's measure.
    var choice: Binding<ListeningSeries.Quantity>?
    @Environment(ViewOptions.self) private var options

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline) {
                    title
                    Spacer(minLength: 12)
                    accessory
                }
                VStack(alignment: .leading, spacing: 2) {
                    title
                    accessory
                }
            }
            if choice != nil {
                peak
            }
            if isFailed {
                SectionFailure()
            } else {
                LampGrid(series: series)
                axis
            }
        }
        .panel()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(titleText)
        .accessibilityValue(isFailed ? failure : spokenSummary)
        .chartSummary(chartSummary)
        .modifier(QuantityAdjustment(choice: choice))
    }

    private var quantity: ListeningSeries.Quantity {
        series?.quantity ?? choice?.wrappedValue ?? .init(options.measure(for: screen))
    }

    private var bucket: ListeningSeries.Bucket { series?.bucket ?? .day }

    private var failure: String { String(localized: "Couldn’t load. Pull down to retry.") }

    private var title: some View {
        Text(titleText)
            .font(.headline)
            .accessibilityAddTraits(.isHeader)
    }

    /// The quantity menu on Stats, the peak elsewhere.
    @ViewBuilder private var accessory: some View {
        if let choice {
            Picker("Count", selection: choice) {
                Text("Plays").tag(ListeningSeries.Quantity.plays)
                Text("Time").tag(ListeningSeries.Quantity.time)
                Text("Artists").tag(ListeningSeries.Quantity.artists)
            }
            .pickerStyle(.menu)
            .tint(.primary)
            .fixedSize()
            .frame(minHeight: 44)
        } else {
            peak
        }
    }

    @ViewBuilder private var peak: some View {
        if let series, let peak = series.peak {
            Text("Peak \(series.label(peak.key)) · \(figure(peak.value))")
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
        }
    }

    /// First, middle and last bucket under the grid.
    @ViewBuilder private var axis: some View {
        let columns = series?.columns ?? []
        HStack {
            ForEach(Array(tickIndices(columns.count).enumerated()), id: \.element) { position, index in
                if position > 0 { Spacer(minLength: 8) }
                Text(series?.tick(columns[index].key) ?? "")
            }
        }
        .font(.caption2.monospacedDigit())
        .foregroundStyle(.secondary)
        .lineLimit(1)
        .frame(minHeight: 14)
    }

    private func tickIndices(_ count: Int) -> [Int] {
        switch count {
        case 0: []
        case 1: [0]
        case 2: [0, 1]
        default: [0, count / 2, count - 1]
        }
    }

    private func figure(_ value: Int) -> String {
        quantity == .time ? Figures.duration(milliseconds: value) : Figures.count(value)
    }

    private func spoken(_ value: Int) -> String {
        switch quantity {
        case .plays: Spoken.plays(value)
        case .time: Figures.duration(milliseconds: value, width: .wide)
        case .artists: String(localized: "\(value) artists")
        }
    }

    // MARK: - Words

    private var titleText: String {
        switch (quantity, bucket) {
        case (.plays, .hour): String(localized: "Plays by hour")
        case (.plays, .day): String(localized: "Plays by day")
        case (.plays, .week): String(localized: "Plays by week")
        case (.plays, .month): String(localized: "Plays by month")
        case (.plays, .year): String(localized: "Plays by year")
        case (.time, .hour): String(localized: "Time by hour")
        case (.time, .day): String(localized: "Time by day")
        case (.time, .week): String(localized: "Time by week")
        case (.time, .month): String(localized: "Time by month")
        case (.time, .year): String(localized: "Time by year")
        case (.artists, .hour): String(localized: "Artists by hour")
        case (.artists, .day): String(localized: "Artists by day")
        case (.artists, .week): String(localized: "Artists by week")
        case (.artists, .month): String(localized: "Artists by month")
        case (.artists, .year): String(localized: "Artists by year")
        }
    }

    /// "Average 35 plays a day. Peak Wed, Oct 7, 176 plays."
    private var spokenSummary: String {
        guard let series, let peak = series.peak else {
            return String(localized: "No plays in this range.")
        }
        let average = spoken(Int(series.average.rounded()))
        let averageText =
            switch bucket {
            case .hour: String(localized: "Average \(average) an hour.")
            case .day: String(localized: "Average \(average) a day.")
            case .week: String(localized: "Average \(average) a week.")
            case .month: String(localized: "Average \(average) a month.")
            case .year: String(localized: "Average \(average) a year.")
            }
        let peakText = String(localized: "Peak \(spokenBucket(peak.key, in: series)), \(spoken(peak.value)).")
        return "\(averageText) \(peakText)"
    }

    private func spokenBucket(_ key: String, in series: ListeningSeries) -> String {
        let label = series.label(key)
        return series.bucket == .week ? String(localized: "week of \(label)") : label
    }

    private var chartSummary: ChartSummary? {
        guard let series, !series.isEmpty else { return nil }
        var lines = [ChartSummary.Series(name: titleText, values: series.columns.map { Double($0.value) })]
        if series.hasPrevious {
            lines.append(
                ChartSummary.Series(
                    name: String(localized: "Period before"), values: series.columns.map { $0.previous.map(Double.init) }))
        }
        return ChartSummary(
            title: titleText, summary: spokenSummary, xTitle: bucket.axisTitle, yTitle: quantity.label,
            categories: series.columns.map { spokenBucket($0.key, in: series) }, series: lines,
            range: 0...Double(max(1, series.scale)), format: quantity == .time ? .duration : .count)
    }
}

/// On Stats, VoiceOver swipes up and down to count plays, time or artists.
private struct QuantityAdjustment: ViewModifier {
    let choice: Binding<ListeningSeries.Quantity>?

    func body(content: Content) -> some View {
        if let choice {
            content.accessibilityAdjustableAction { direction in
                let all = ListeningSeries.Quantity.allCases
                guard let index = all.firstIndex(of: choice.wrappedValue) else { return }
                switch direction {
                case .increment: choice.wrappedValue = all[(index + 1) % all.count]
                case .decrement: choice.wrappedValue = all[(index + all.count - 1) % all.count]
                @unknown default: break
                }
            }
        } else {
            content
        }
    }
}

extension ListeningSeries.Bucket {
    /// The axis a bucket runs along, for the audio graph.
    var axisTitle: String {
        switch self {
        case .hour: String(localized: "Hour")
        case .day: String(localized: "Day")
        case .week: String(localized: "Week")
        case .month: String(localized: "Month")
        case .year: String(localized: "Year")
        }
    }
}

extension ListeningSeries.Quantity {
    var label: String {
        switch self {
        case .plays: String(localized: "Plays")
        case .time: String(localized: "Time")
        case .artists: String(localized: "Artists")
        }
    }
}
