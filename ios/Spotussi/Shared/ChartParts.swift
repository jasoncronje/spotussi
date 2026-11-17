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
