import SpotussiKit
import SwiftUI

/// Horizontal bands from the bottom up, for masking a chart's columns into
/// lamp segments like the web's share columns.
struct LampStripes: View {
    var body: some View {
        Canvas { context, size in
            var bottom = size.height
            while bottom > 0 {
                context.fill(Path(CGRect(x: 0, y: bottom - 4, width: size.width, height: 4)), with: .color(.black))
                bottom -= 6
            }
        }
    }
}

/// An empty plot while a chart's first answer is on its way.
struct ChartPlaceholder: View {
    let height: CGFloat

    var body: some View {
        Rectangle()
            .fill(.grille)
            .mask { LampStripes() }
            .frame(height: height)
            .accessibilityHidden(true)
    }
}

enum ChartAxis {
    /// The first, middle and last keys: labels that never crowd, at any
    /// text size.
    static func keys(_ keys: [String]) -> [String] {
        switch keys.count {
        case 0...2: keys
        default: [keys[0], keys[keys.count / 2], keys[keys.count - 1]]
        }
    }
}

/// Date labels under a chart with a String x axis, on the given keys only.
///
/// Swift Charts drops explicit `AxisMarks(values:)` on a categorical axis
/// once the plot is wide enough (an iPhone Duo's inner display), labelling
/// every column so the dates run together. So the axis is hidden and the
/// labels are drawn here from the chart's own positions: the first starts at
/// the plot's leading edge, the last ends at its trailing edge, any between
/// centres on its column.
struct ChartTickLabels: ViewModifier {
    let keys: [String]
    let label: (String) -> String
    @ScaledMetric(relativeTo: .caption2) private var rowHeight: CGFloat = 18

    func body(content: Content) -> some View {
        content
            .chartXAxis(.hidden)
            .chartOverlay(alignment: .topLeading) { proxy in
                GeometryReader { geometry in
                    if let anchor = proxy.plotFrame {
                        let plot = geometry[anchor]
                        ForEach(Array(keys.enumerated()), id: \.offset) { index, key in
                            if let x = proxy.position(forX: key) {
                                tick(label(key), index: index, x: plot.minX + x, plot: plot)
                            }
                        }
                    }
                }
            }
            .padding(.bottom, rowHeight)
    }

    @ViewBuilder
    private func tick(_ text: String, index: Int, x: CGFloat, plot: CGRect) -> some View {
        let y = plot.maxY + rowHeight / 2 + 2
        let label = Text(text)
            .font(.caption2)
            .foregroundStyle(.secondary)
            .lineLimit(1)
            .fixedSize()
        if keys.count > 1 && index == 0 {
            label.frame(width: plot.width, alignment: .leading).position(x: plot.midX, y: y)
        } else if keys.count > 1 && index == keys.count - 1 {
            label.frame(width: plot.width, alignment: .trailing).position(x: plot.midX, y: y)
        } else {
            label.position(x: x, y: y)
        }
    }
}

extension View {
    /// Labels a chart's x axis on `keys` only; see ``ChartTickLabels``.
    func chartTickLabels(_ keys: [String], label: @escaping (String) -> String) -> some View {
        modifier(ChartTickLabels(keys: keys, label: label))
    }
}

/// A chart's title, with a figure or a reading on the right, stacked when
/// they don't fit side by side.
struct ChartHeader<Trailing: View>: View {
    let title: String
    @ViewBuilder let trailing: Trailing

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline) {
                titleText
                Spacer(minLength: 12)
                trailing
            }
            VStack(alignment: .leading, spacing: 2) {
                titleText
                trailing
            }
        }
    }

    private var titleText: some View {
        Text(title)
            .font(.headline)
            .accessibilityAddTraits(.isHeader)
    }
}
