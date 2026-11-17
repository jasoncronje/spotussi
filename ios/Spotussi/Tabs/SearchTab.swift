import SpotussiKit
import SwiftUI

/// Placeholder until I3 wires the field to `/search`.
struct SearchTab: View {
    @State private var text = ""

    var body: some View {
        TabPage(title: "Search", controls: .settingsOnly) {
            ComingNextPanel(
                symbol: "magnifyingglass",
                title: "Search what you’ve played",
                message: "Find any track, artist or album in your history. Coming in the next update.")
        }
        .searchable(text: $text, prompt: "Tracks, artists, albums")
    }
}

#Preview {
    PreviewHost(.demo) { SearchTab() }
}
