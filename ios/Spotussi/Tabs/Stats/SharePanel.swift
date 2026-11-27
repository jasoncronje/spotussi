import Charts
import SpotussiKit
import SwiftUI

/// How much of each bucket a few artists or genres hold, as the web's
/// share columns: stacked lamp segments, the leader lit, the rest in steps
/// of aluminium, and what they don't hold left unlit. Scrubbing a column
/// shows its date and total, and the legend switches to its shares. The
/// legend's names open their pages.
struct SharePanel: View {
    let title: String
    let kind: EntityKind
    let breakdown: ShareBreakdown?
    let isFailed: Bool
    @State private var selected: String?
    @Environment(\.colorScheme) private var scheme
    /// Grows with the text so the axis labels keep their room, within reason.
    @ScaledMetric(relativeTo: .caption2) private var scaledHeight: CGFloat = 180

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ChartHeader(title: title) { reading }
            if isFailed {
                SectionFailure()
            } else if let breakdown, breakdown.series.isEmpty || breakdown.total == 0 {
                (kind == .genre ? Text("No genres yet.") : Text("No plays in this range."))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else if let breakdown {
                chart(breakdown)
                legend(breakdown)
            } else {
                ChartPlaceholder(height: chartHeight)
                placeholderLegend
            }
        }
        .panel()
    }

    // MARK: - Reading

    private var column: ShareBreakdown.Column? {
        guard let selected else { return nil }
        return breakdown?.columns.first { $0.key == selected }
    }

    /// "Top 5 · 38%", or the scrubbed column's date and total.
    @ViewBuilder private var reading: some View {
        if let breakdown, !breakdown.series.isEmpty {
            Group {
                if let column {
                    Text("\(breakdown.bucket.label(column.key)) · \(figure(column.total, breakdown.measure))")
                } else {
                    Text("Top \(breakdown.series.count) · \(Figures.percent(min(1, breakdown.held)))")
                }
            }
            .font(.caption.monospacedDigit())
            .foregroundStyle(.secondary)
            .accessibilityHidden(true)
        }
    }

    private var chartHeight: CGFloat { min(scaledHeight, 300) }

    // MARK: - Chart

    private func chart(_ breakdown: ShareBreakdown) -> some View {
        let colors = Tone.series.map { $0.color(for: scheme) }
        let grille = Tone.grille.color(for: scheme)
        let scale = breakdown.scale
        let ticks = ChartAxis.keys(breakdown.columns.map(\.key))
        return Chart {
            ForEach(breakdown.columns) { column in
                let dimmed = selected != nil && selected != column.key
                BarMark(
                    x: .value("Bucket", column.key), yStart: .value("Share", 0.0), yEnd: .value("Share", scale),
                    width: .ratio(0.72)
                )
                .foregroundStyle(grille)
                .opacity(dimmed ? 0.45 : 1)
                ForEach(breakdown.segments(column)) { segment in
                    BarMark(
                        x: .value("Bucket", column.key), yStart: .value("Share", segment.start),
                        yEnd: .value("Share", min(segment.end, scale)), width: .ratio(0.72)
                    )
                    .foregroundStyle(colors[segment.series % colors.count])
                    .opacity(dimmed ? 0.45 : 1)
                }
            }
        }
        .chartYScale(domain: 0...scale)
        .chartXAxis {
            // Marks on the chosen few only. Given string values, Charts
            // labelled every column, and leaving the label out of a mark
            // didn't stop it: the default label came back and the dates ran
            // together into one unreadable line. Explicit values do.
            AxisMarks(values: ticks) { value in
                AxisValueLabel {
                    if let key = value.as(String.self) {
                        Text(breakdown.bucket.tick(key))
                    }
                }
            }
        }
        .chartYAxis {
            AxisMarks(position: .leading, values: [0, scale / 2, scale]) { value in
                AxisValueLabel {
                    if let share = value.as(Double.self) {
                        Text(Figures.percent(share))
                    }
                }
            }
        }
        .chartXSelection(value: $selected)
        .chartPlotStyle { plot in
            plot.mask { LampStripes() }
        }
        .frame(height: chartHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(title)
        .accessibilityValue(spokenSummary(breakdown))
        .chartSummary(chartSummary(breakdown))
    }

    // MARK: - Legend

    /// Each series in stack order, the leader first, with its share of the
    /// range or of the scrubbed column, then what no series holds.
    private func legend(_ breakdown: ShareBreakdown) -> some View {
        let colors = Tone.series.map { $0.color(for: scheme) }
        let column = column
        return VStack(spacing: 0) {
            ForEach(breakdown.series) { series in
                let share = column.map { $0.shares[series.id] } ?? series.share
                RouteLink(
                    route: .entity(series.link), accessibilityLabel: series.link.name,
                    accessibilityValue: Figures.percent(share)
                ) {
                    LegendRow(color: colors[series.id % colors.count], name: series.link.name, share: share)
                }
            }
            let other = column?.other ?? max(0, 1 - breakdown.held)
            LegendRow(color: Tone.grille.color(for: scheme), name: otherName, share: other)
                .foregroundStyle(.secondary)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(otherName)
                .accessibilityValue(Figures.percent(other))
        }
    }

    private var placeholderLegend: some View {
        VStack(spacing: 0) {
            ForEach(0..<3, id: \.self) { index in
                LegendRow(color: Tone.grille.color(for: scheme), name: "Placeholder name", share: 0.1)
            }
        }
        .redacted(reason: .placeholder)
        .accessibilityHidden(true)
    }

    private var otherName: String {
        kind == .genre ? String(localized: "Other genres") : String(localized: "Everyone else")
    }

    // MARK: - Words

    private func figure(_ value: Int, _ measure: Measure) -> String {
        measure == .plays ? Spoken.plays(value) : Figures.duration(milliseconds: value)
    }

    /// "Top 5 artists hold 38% of plays." or "modern pop leads with 12% of
    /// time."
    private func spokenSummary(_ breakdown: ShareBreakdown) -> String {
        let held = Figures.percent(min(1, breakdown.held))
        if kind == .genre, let first = breakdown.series.first {
            let share = Figures.percent(first.share)
            return breakdown.measure == .plays
                ? String(localized: "\(first.link.name) leads with \(share) of plays.")
                : String(localized: "\(first.link.name) leads with \(share) of time.")
        }
        let count = breakdown.series.count
        return breakdown.measure == .plays
            ? String(localized: "Top \(count) artists hold \(held) of plays.")
            : String(localized: "Top \(count) artists hold \(held) of time.")
    }

    private func chartSummary(_ breakdown: ShareBreakdown) -> ChartSummary {
        ChartSummary(
            title: title, summary: spokenSummary(breakdown), xTitle: breakdown.bucket.axisTitle,
            yTitle: String(localized: "Share"),
            categories: breakdown.columns.map { breakdown.bucket.label($0.key) },
            series: breakdown.series.map { series in
                ChartSummary.Series(name: series.link.name, values: breakdown.columns.map { $0.shares[series.id] })
            },
            range: 0...breakdown.scale, format: .percent)
    }
}

/// A swatch, a name and a share.
private struct LegendRow: View {
    let color: Color
    let name: String
    let share: Double

    var body: some View {
        HStack(spacing: 10) {
            RoundedRectangle(cornerRadius: 2)
                .fill(color)
                .frame(width: 10, height: 10)
            Text(name)
                .font(.subheadline)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(Figures.percent(share))
                .font(.subheadline.monospacedDigit())
                .foregroundStyle(.secondary)
                .fixedSize()
        }
        .frame(minHeight: 44)
        .contentShape(.rect)
    }
}
