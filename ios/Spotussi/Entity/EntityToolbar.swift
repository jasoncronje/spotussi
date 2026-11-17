import SpotussiKit
import SwiftUI

extension View {
    /// A pushed page's controls: the shared range beside the back button,
    /// this kind of page's Plays/Time, and Open in Spotify when Spotify
    /// knows the entity.
    func entityToolbar(screen: ViewOptions.Screen, spotifyURI: String? = nil) -> some View {
        modifier(EntityToolbar(screen: screen, spotifyURL: spotifyURI.flatMap(SpotifyLink.url(for:))))
    }
}

private struct EntityToolbar: ViewModifier {
    let screen: ViewOptions.Screen
    let spotifyURL: URL?
    @Environment(\.openURL) private var openURL

    func body(content: Content) -> some View {
        content
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    RangeMenu()
                }
                ToolbarItem(placement: .topBarTrailing) {
                    MeasurePicker(screen: screen)
                }
                if let spotifyURL {
                    ToolbarSpacer(.fixed, placement: .topBarTrailing)
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            openURL(spotifyURL)
                        } label: {
                            Label("Open in Spotify", systemImage: "arrow.up.forward.app")
                        }
                    }
                }
            }
    }
}
