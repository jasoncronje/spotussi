import SpotussiKit
import SwiftUI

/// Opens as a sheet from the toolbar. Placeholder until I3 wires the field
/// to `/search`.
struct SearchTab: View {
    @State private var text = ""
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ScrollView {
                ComingNextPanel(
                    symbol: "magnifyingglass",
                    title: "Search what you’ve played",
                    message: "Coming soon.")
                .padding(16)
            }
            .background(.ground)
            .navigationTitle("Search")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $text, prompt: "Tracks, artists, albums")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(role: .close) { dismiss() }
                }
            }
        }
    }
}

#Preview {
    PreviewHost(.demo) { SearchTab() }
}
