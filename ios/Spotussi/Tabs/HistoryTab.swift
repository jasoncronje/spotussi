import SpotussiKit
import SwiftUI

/// Placeholder until I4 brings plays and sessions.
struct HistoryTab: View {
    @Environment(ViewOptions.self) private var options

    var body: some View {
        TabPage(title: "History", subtitle: Text(options.range.longLabel), controls: .rangeOnly) {
            ComingNextPanel(
                symbol: "clock.arrow.circlepath",
                title: "Every play, newest first",
                message: "With filters for skips, shuffle and devices, and your longest sessions. Coming in a later update.")
        }
    }
}

#Preview {
    PreviewHost(.demo) { HistoryTab() }
}
