import Accessibility
import SpotussiKit
import SwiftUI

/// Plays (or time) per bucket of the range on the lamp grid, with the peak
/// above it and dates below. VoiceOver reads a summary and can play the
/// series as an audio graph.
struct PlaysByDayPanel: View {
    let series: ListeningSeries?
    let isFailed: Bool
    /// Whose Plays/Time choice titles the panel before the series is in.
    let screen: ViewOptions.Screen
    @Environment(ViewOptions.self) private var options

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline) {
                    title
                    Spacer(minLength: 12)
                    peak
                }
                VStack(alignment: .leading, spacing: 2) {
                    title
                    peak
                }
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
        .accessibilityValue(isFailed ? Text("Couldn’t load. Pull down to retry.") : Text(spokenSummary))
        .modifier(ChartDescription(descriptor: descriptor))
    }

    private var measure: Measure { series?.measure ?? options.measure(for: screen) }
    private var bucket: ListeningSeries.Bucket { series?.bucket ?? .day }

    private var title: some View {
        Text(titleText)
            .font(.headline)
            .accessibilityAddTraits(.isHeader)
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
        measure == .plays ? Figures.count(value) : Figures.duration(milliseconds: value)
    }

    // MARK: - Words

    private var titleText: String {
        switch (measure, bucket) {
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
        }
    }

    /// "Average 35 plays a day. Peak Wed, Oct 7, 176 plays."
    private var spokenSummary: String {
        guard let series, let peak = series.peak else {
            return String(localized: "No plays in this range.")
        }
        let average = Spoken.value(Int(series.average.rounded()), measure: measure)
        let averageText =
            switch bucket {
            case .hour: String(localized: "Average \(average) an hour.")
            case .day: String(localized: "Average \(average) a day.")
            case .week: String(localized: "Average \(average) a week.")
            case .month: String(localized: "Average \(average) a month.")
            case .year: String(localized: "Average \(average) a year.")
            }
        let peakText = String(
            localized: "Peak \(spokenBucket(peak.key, in: series)), \(Spoken.value(peak.value, measure: measure)).")
        return "\(averageText) \(peakText)"
    }

    private func spokenBucket(_ key: String, in series: ListeningSeries) -> String {
        let label = series.label(key)
        return series.bucket == .week ? String(localized: "week of \(label)") : label
    }

    private var descriptor: LampGridDescriptor? {
        guard let series, !series.isEmpty else { return nil }
        let axisTitle =
            switch bucket {
            case .hour: String(localized: "Hour")
            case .day: String(localized: "Day")
            case .week: String(localized: "Week")
            case .month: String(localized: "Month")
            case .year: String(localized: "Year")
            }
        return LampGridDescriptor(
            title: titleText, summary: spokenSummary, xTitle: axisTitle, yTitle: measure.label,
            previousTitle: String(localized: "Period before"),
            categories: series.columns.map { spokenBucket($0.key, in: series) },
            values: series.columns.map { Double($0.value) },
            previous: series.hasPrevious ? series.columns.map { $0.previous.map(Double.init) } : nil,
            maximum: Double(series.scale), isTime: measure == .time)
    }
}

/// Adds the audio graph only once there is a series to play.
private struct ChartDescription: ViewModifier {
    let descriptor: LampGridDescriptor?

    func body(content: Content) -> some View {
        if let descriptor {
            content.accessibilityChartDescriptor(descriptor)
        } else {
            content
        }
    }
}

/// The series for VoiceOver's audio graph: one data point per bucket, and
/// the period before as a second series when there is one.
private nonisolated struct LampGridDescriptor: AXChartDescriptorRepresentable {
    let title: String
    let summary: String
    let xTitle: String
    let yTitle: String
    let previousTitle: String
    let categories: [String]
    let values: [Double]
    let previous: [Double?]?
    let maximum: Double
    let isTime: Bool

    func makeChartDescriptor() -> AXChartDescriptor {
        let isTime = isTime
        let xAxis = AXCategoricalDataAxisDescriptor(title: xTitle, categoryOrder: categories)
        let yAxis = AXNumericDataAxisDescriptor(
            title: yTitle, range: 0...max(1, maximum), gridlinePositions: []
        ) { value in
            isTime ? Figures.duration(milliseconds: Int(value), width: .wide) : Figures.count(Int(value))
        }
        var series = [
            AXDataSeriesDescriptor(
                name: title, isContinuous: false,
                dataPoints: zip(categories, values).map { AXDataPoint(x: $0, y: $1) })
        ]
        if let previous {
            series.append(
                AXDataSeriesDescriptor(
                    name: previousTitle, isContinuous: false,
                    dataPoints: zip(categories, previous).map { AXDataPoint(x: $0, y: $1) }))
        }
        return AXChartDescriptor(
            title: title, summary: summary, xAxis: xAxis, yAxis: yAxis, additionalAxes: [], series: series)
    }
}
