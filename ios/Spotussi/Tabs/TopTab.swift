import SpotussiKit
import SwiftUI

/// Placeholder until I3 brings the four ranked lists.
struct TopTab: View {
    @Environment(ViewOptions.self) private var options

    var body: some View {
        let measure = options.measure(for: .top)
        TabPage(
            title: "Top",
            subtitle: measure == .plays
                ? Text("\(options.range.longLabel) · by plays")
                : Text("\(options.range.longLabel) · by time"),
            controls: .rangeAndMeasure(.top)
        ) {
            ComingNextPanel(
                symbol: "list.number",
                title: "Top tracks, artists, albums and genres",
                message: measure == .plays
                    ? "Coming soon."
                    : "Coming soon.")
        }
    }
}

#Preview {
    PreviewHost(.demo) { TopTab() }
}
