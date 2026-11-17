import SpotussiKit
import SwiftUI

/// Placeholder until I4 brings the charts and the listening clock.
struct StatsTab: View {
    @Environment(ViewOptions.self) private var options

    var body: some View {
        TabPage(
            title: "Stats", subtitle: Text(options.range.longLabel),
            controls: .rangeAndMeasure(.stats)
        ) {
            ComingNextPanel(
                symbol: "chart.bar.xaxis",
                title: "Charts and the listening clock",
                message: "Coming soon.")
        }
    }
}

#Preview {
    PreviewHost(.demo) { StatsTab() }
}
