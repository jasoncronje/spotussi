import SpotussiKit
import SwiftUI

/// A row that opens a route: the whole row is the target, it darkens while
/// pressed, and VoiceOver reads it as one button with the row's words.
struct RouteLink<Label: View>: View {
    let route: Route
    let accessibilityLabel: String
    var accessibilityValue: String?
    /// A Spotify URI for the row's "Open in Spotify" menu.
    var spotifyURI: String?
    @ViewBuilder let label: Label

    @Environment(\.openURL) private var openURL

    var body: some View {
        NavigationLink(value: route) {
            label
        }
        .buttonStyle(RowButtonStyle())
        .contextMenu {
            if let url = spotifyURI.flatMap(SpotifyLink.url(for:)) {
                Button("Open in Spotify", systemImage: "arrow.up.forward.app") {
                    openURL(url)
                }
            }
        }
        .accessibilityLabel(accessibilityLabel)
        .accessibilityValue(accessibilityValue ?? "")
    }
}

/// A row on a panel: no tint, and a soft highlight under it while pressed,
/// reaching a little past the row's own edges.
struct RowButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(.primary)
            .padding(.horizontal, 8)
            .background(
                Color.primary.opacity(configuration.isPressed ? 0.07 : 0), in: .rect(cornerRadius: 12)
            )
            .padding(.horizontal, -8)
            .contentShape(.rect)
    }
}
