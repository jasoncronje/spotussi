import Accessibility
import SpotussiKit
import SwiftUI

/// A chart for VoiceOver's audio graph: one data point per category, in
/// one or more series, with values spoken the way the screen writes them.
nonisolated struct ChartSummary: AXChartDescriptorRepresentable {
    enum Format: Sendable {
        case count, duration, percent, year, ratio
    }

    struct Series: Sendable {
        let name: String
        let values: [Double?]
    }

    let title: String
    let summary: String
    let xTitle: String
    let yTitle: String
    let categories: [String]
    let series: [Series]
    let range: ClosedRange<Double>
    let format: Format

    func makeChartDescriptor() -> AXChartDescriptor {
        let format = format
        let xAxis = AXCategoricalDataAxisDescriptor(title: xTitle, categoryOrder: categories)
        let yAxis = AXNumericDataAxisDescriptor(
            title: yTitle, range: range.lowerBound...max(range.upperBound, range.lowerBound + 1), gridlinePositions: []
        ) { value in
            Self.spoken(value, format: format)
        }
        let series = series.map { series in
            AXDataSeriesDescriptor(
                name: series.name, isContinuous: false,
                dataPoints: zip(categories, series.values).map { AXDataPoint(x: $0, y: $1) })
        }
        return AXChartDescriptor(
            title: title, summary: summary, xAxis: xAxis, yAxis: yAxis, additionalAxes: [], series: series)
    }

    static func spoken(_ value: Double, format: Format) -> String {
        switch format {
        case .count: Figures.count(Int(value.rounded()))
        case .duration: Figures.duration(milliseconds: Int(value), width: .wide)
        case .percent: Figures.percent(value)
        case .year: String(Int(value.rounded()))
        case .ratio: value.formatted(.number.precision(.fractionLength(2)))
        }
    }
}

extension View {
    /// Adds the audio graph once there is something to play.
    func chartSummary(_ summary: ChartSummary?) -> some View {
        modifier(ChartSummaryModifier(summary: summary))
    }
}

private struct ChartSummaryModifier: ViewModifier {
    let summary: ChartSummary?

    func body(content: Content) -> some View {
        if let summary {
            content.accessibilityChartDescriptor(summary)
        } else {
            content
        }
    }
}
