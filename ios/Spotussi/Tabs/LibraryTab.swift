import SpotussiKit
import SwiftUI

/// Placeholder until I5. No range or measure: saves carry their own dates (D36).
struct LibraryTab: View {
    var body: some View {
        TabPage(title: "Library", controls: .settingsOnly) {
            ComingNextPanel(
                symbol: "books.vertical",
                title: "Saved songs, albums and playlists",
                message: "Coming soon.")
        }
    }
}

#Preview {
    PreviewHost(.demo) { LibraryTab() }
}
