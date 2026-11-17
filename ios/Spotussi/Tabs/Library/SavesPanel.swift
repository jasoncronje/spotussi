import SpotussiKit
import SwiftUI

/// Songs saved over the years on the lamp grid, as the web's saves chart:
/// a column per month, quarter or year, whichever keeps columns apart at
/// this width, lit to the busiest one, and removals as dim lamps under the
/// line (known only since sync started). VoiceOver reads a summary and can
/// play the columns as an audio graph.
struct SavesPanel: View {
    let timeline: SavesTimeline?
    let isFailed: Bool
    @State private var width: CGFloat = 0

    private static let pitch: CGFloat = 9
    private static var height: CGFloat { CGFloat(SavesTimeline.rows + SavesTimeline.removalRows) * pitch + 2 }

    var body: some View {
        let columns = timeline.map { $0.columns(fitting: Double(width)) } ?? []
        VStack(alignment: .leading, spacing: 12) {
            ChartHeader(title: String(localized: "Songs saved")) { reading }
            if isFailed {
                SectionFailure()
            } else if timeline == nil {
                ChartPlaceholder(height: Self.height)
            } else if timeline?.isEmpty == true {
                Text("No saves yet.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                grid(columns)
                axis(columns)
            }
        }
        .panel()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "Songs saved"))
        .accessibilityValue(isFailed ? String(localized: "Couldn’t load. Pull down to retry.") : spokenSummary(columns))
        .chartSummary(chartSummary(columns))
    }

    /// "1,454 saved · 12 removed"
    @ViewBuilder private var reading: some View {
        if let timeline, !timeline.isEmpty {
            Group {
                if timeline.removed > 0 {
                    Text("\(timeline.saved) saved · \(timeline.removed) removed")
                } else {
                    Text("\(timeline.saved) saved")
                }
            }
            .font(.caption.monospacedDigit())
            .foregroundStyle(.secondary)
        }
    }

    // MARK: - Grid

    private func grid(_ columns: [SavesTimeline.Column]) -> some View {
        let peak = columns.map(\.saved).max() ?? 1
        let peakRemoved = columns.map(\.removed).max() ?? 1
        let rows = SavesTimeline.rows
        let pitch = Self.pitch
        return Canvas { context, size in
            guard !columns.isEmpty else { return }
            let columnWidth = size.width / CGFloat(columns.count)
            let side = max(2.8, min(5.2, columnWidth * 0.6))
            let base = CGFloat(rows) * pitch
            for (index, column) in columns.enumerated() {
                let x = (CGFloat(index) + 0.5) * columnWidth
                for (row, brightness) in SavesTimeline.lamps(column, peak: peak).enumerated() {
                    let y = base - (CGFloat(row) + 0.5) * pitch
                    let lamp = Path(ellipseIn: CGRect(x: x - side / 2, y: y - side / 2, width: side, height: side))
                    context.fill(lamp, with: .style(Tone.grille))
                    if brightness > 0 {
                        var lit = context
                        lit.opacity = brightness
                        lit.fill(lamp, with: .style(Tone.lamp))
                    }
                }
                for row in 0..<SavesTimeline.removalLamps(column, peak: peakRemoved) {
                    let y = base + 2 + (CGFloat(row) + 0.5) * pitch
                    var gone = context
                    gone.opacity = 0.55
                    gone.fill(
                        Path(ellipseIn: CGRect(x: x - side / 2, y: y - side / 2, width: side, height: side)),
                        with: .style(HierarchicalShapeStyle.secondary))
                }
            }
            context.fill(Path(CGRect(x: 0, y: base, width: size.width, height: 1)), with: .style(Tone.grille))
        }
        .frame(height: Self.height)
        .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { width = $0 }
        .accessibilityHidden(true)
    }

    /// The first, middle and last years under the grid.
    private func axis(_ columns: [SavesTimeline.Column]) -> some View {
        let years = Array(Set(columns.map(\.year))).sorted()
        let ticks =
            years.count <= 2 ? years : [years[0], years[years.count / 2], years[years.count - 1]]
        return HStack {
            ForEach(Array(ticks.enumerated()), id: \.element) { position, year in
                if position > 0 { Spacer(minLength: 8) }
                Text(verbatim: String(year))
            }
        }
        .font(.caption2.monospacedDigit())
        .foregroundStyle(.secondary)
        .lineLimit(1)
        .frame(minHeight: 14)
    }

    // MARK: - Words

    /// "Mar 2024", "Q1 2024", "2024"
    private func label(_ column: SavesTimeline.Column) -> String {
        switch column.months {
        case 12:
            return String(column.year)
        case 3:
            return String(localized: "Q\((column.month - 1) / 3 + 1) \(String(column.year))")
        default:
            var calendar = Calendar(identifier: .gregorian)
            calendar.timeZone = .gmt
            let date = calendar.date(from: DateComponents(year: column.year, month: column.month, day: 1)) ?? .now
            return date.formatted(
                Date.FormatStyle(calendar: calendar, timeZone: .gmt).month(.abbreviated).year())
        }
    }

    /// "1,454 songs saved since 2021. Most in Q3 2024, 120."
    private func spokenSummary(_ columns: [SavesTimeline.Column]) -> String {
        guard let timeline, !timeline.isEmpty, let first = columns.first,
            let busiest = columns.max(by: { $0.saved < $1.saved })
        else { return String(localized: "No saves yet.") }
        let since = String(localized: "\(timeline.saved) songs saved since \(String(first.year)).")
        let most = String(localized: "Most in \(label(busiest)), \(busiest.saved).")
        return "\(since) \(most)"
    }

    private func chartSummary(_ columns: [SavesTimeline.Column]) -> ChartSummary? {
        guard let timeline, !timeline.isEmpty, !columns.isEmpty else { return nil }
        let title = String(localized: "Songs saved")
        var series = [ChartSummary.Series(name: title, values: columns.map { Double($0.saved) })]
        if timeline.removed > 0 {
            series.append(
                ChartSummary.Series(name: String(localized: "Removed"), values: columns.map { Double($0.removed) }))
        }
        return ChartSummary(
            title: title, summary: spokenSummary(columns), xTitle: String(localized: "Date"),
            yTitle: String(localized: "Songs"), categories: columns.map(label), series: series,
            range: 0...Double(columns.map(\.saved).max() ?? 1), format: .count)
    }
}
