import Charts
import SpotussiKit
import SwiftUI

/// A weighted average over time, as the web's average dots: one lamp per
/// bucket at its value, brighter the more of its listening had the detail,
/// joined by a hairline that breaks at empty buckets, and the range's
/// average as a dashed line and the figure in the corner. Scrubbing reads
/// out a bucket.
struct AveragePanel: View {
    let title: String
    let trend: AverageTrend?
    let isFailed: Bool
    /// Said when nothing in the range had the detail.
    let emptyText: LocalizedStringKey
    @State private var selected: String?
    @Environment(\.colorScheme) private var scheme
    /// Grows with the text so the axis labels keep their room, within reason.
    @ScaledMetric(relativeTo: .caption2) private var scaledHeight: CGFloat = 180

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ChartHeader(title: title) { overall }
            if isFailed {
                SectionFailure()
            } else if let trend, trend.overall == nil, trend.known.isEmpty {
                Text(emptyText)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else if let trend {
                reading(trend)
                chart(trend)
            } else {
                ChartPlaceholder(height: chartHeight)
            }
        }
        .panel()
    }

    @ViewBuilder private var overall: some View {
        if let trend, let overall = trend.overall {
            Text(trend.format(overall))
                .font(.readout(.title3))
                .accessibilityHidden(true)
        }
    }

    /// The scrubbed bucket: "Wed, Oct 21 · 2018 · 98% known". A blank line
    /// otherwise, so the chart doesn't jump.
    private func reading(_ trend: AverageTrend) -> some View {
        let point = selected.flatMap { key in trend.points.first { $0.key == key } }
        return Group {
            if let point, let value = point.value {
                Text(
                    "\(trend.bucket.label(point.key)) · \(trend.format(value)) · \(Figures.percent(point.coverage)) known")
            } else {
                Text(verbatim: " ")
            }
        }
        .font(.caption.monospacedDigit())
        .foregroundStyle(.secondary)
        .accessibilityHidden(true)
    }

    private var chartHeight: CGFloat { min(scaledHeight, 300) }

    // MARK: - Chart

    private struct LinePoint: Identifiable {
        let id: Int
        let run: Int
        let key: String
        let value: Double
    }

    private func chart(_ trend: AverageTrend) -> some View {
        let lamp = Tone.lamp.color(for: scheme)
        let scale = trend.scale
        let ticks = ChartAxis.keys(trend.points.map(\.key))
        let line = trend.runs.enumerated().flatMap { run, points in
            points.map { LinePoint(id: $0.id, run: run, key: $0.key, value: $0.value ?? 0) }
        }
        return Chart {
            if let overall = trend.overall {
                RuleMark(y: .value("Average", overall))
                    .foregroundStyle(lamp)
                    .lineStyle(StrokeStyle(lineWidth: 1.5, dash: [2, 4]))
            }
            ForEach(line) { point in
                LineMark(
                    x: .value("Bucket", point.key), y: .value("Value", point.value), series: .value("Run", point.run)
                )
                .foregroundStyle(Color.primary.opacity(0.3))
                .lineStyle(StrokeStyle(lineWidth: 1))
            }
            ForEach(trend.known) { point in
                PointMark(x: .value("Bucket", point.key), y: .value("Value", point.value ?? 0))
                    .foregroundStyle(lamp.opacity(0.3 + 0.7 * point.coverage))
                    .symbolSize(selected == point.key ? 90 : 36)
            }
        }
        .chartXScale(domain: trend.points.map(\.key))
        .chartYScale(domain: scale.domain)
        .chartXAxis {
            // Labels on the chosen few only; see SharePanel.
            AxisMarks(values: ticks) { value in
                AxisValueLabel {
                    if let key = value.as(String.self) {
                        Text(trend.bucket.tick(key))
                    }
                }
            }
        }
        .chartYAxis {
            AxisMarks(position: .leading, values: scale.ticks) { value in
                AxisGridLine()
                AxisValueLabel {
                    if let tick = value.as(Double.self) {
                        Text(trend.format(tick))
                    }
                }
            }
        }
        .chartXSelection(value: $selected)
        .frame(height: chartHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(title)
        .accessibilityValue(spokenSummary(trend))
        .chartSummary(chartSummary(trend))
    }

    // MARK: - Words

    /// "Average 2019. 2 years newer at the end." or "Average 1.26."
    private func spokenSummary(_ trend: AverageTrend) -> String {
        guard let overall = trend.overall else { return "" }
        let average = String(localized: "Average \(trend.format(overall)).")
        guard trend.kind == .year, let change = trend.change, change != 0 else { return average }
        let years = Int(abs(change))
        let direction =
            change > 0
            ? String(localized: "\(years) years newer at the end.")
            : String(localized: "\(years) years older at the end.")
        return "\(average) \(direction)"
    }

    private func chartSummary(_ trend: AverageTrend) -> ChartSummary {
        ChartSummary(
            title: title, summary: spokenSummary(trend), xTitle: trend.bucket.axisTitle, yTitle: title,
            categories: trend.points.map { trend.bucket.label($0.key) },
            series: [ChartSummary.Series(name: title, values: trend.points.map(\.value))],
            range: trend.scale.domain, format: trend.kind == .year ? .year : .ratio)
    }
}
