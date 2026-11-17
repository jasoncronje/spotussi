import SpotussiAPI
import SpotussiKit
import SwiftUI

extension View {
    /// The playing track in the tab bar's accessory, there only while
    /// something is playing or paused.
    func nowPlayingAccessory(_ model: NowPlayingModel) -> some View {
        modifier(NowPlayingAccessoryModifier(model: model))
    }
}

private struct NowPlayingAccessoryModifier: ViewModifier {
    let model: NowPlayingModel

    func body(content: Content) -> some View {
        if #available(iOS 26.1, *) {
            content.tabViewBottomAccessory(isEnabled: model.track != nil) { accessory }
        } else {
            // iOS 26.0 can't hide the accessory, and adding it only when
            // needed would rebuild every tab, so it says nothing is playing.
            content.tabViewBottomAccessory { accessory }
        }
    }

    @ViewBuilder private var accessory: some View {
        if let nowPlaying = model.nowPlaying, let track = nowPlaying.track {
            NowPlayingBar(track: track, isPlaying: nowPlaying.isPlaying)
        } else {
            Text("Nothing playing")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
        }
    }
}

/// Artwork, track and artists, and a lamp that is lit while playing. In
/// the minimised tab bar it keeps to one line.
private struct NowPlayingBar: View {
    let track: Components.Schemas.TrackRef
    let isPlaying: Bool
    @Environment(\.tabViewBottomAccessoryPlacement) private var placement

    var body: some View {
        HStack(spacing: 10) {
            ArtworkImage(url: track.imageUrl, side: isInline ? 24 : 30, symbol: "music.note")
            VStack(alignment: .leading, spacing: 0) {
                Text(track.name)
                    .font(.subheadline.weight(.semibold))
                if !isInline, let artists {
                    Text(artists)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
            PlayingIndicator(isPlaying: isPlaying)
        }
        .padding(.horizontal, 14)
        // The accessory's height is the system's; past this size the
        // words would be cut off, and VoiceOver reads them in full.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(isPlaying ? Text("Now playing") : Text("Paused"))
        .accessibilityValue([track.name, artists].compactMap(\.self).joined(separator: ", "))
    }

    private var isInline: Bool { placement == .inline }

    private var artists: String? {
        track.artists.isEmpty ? nil : track.artists.map(\.name).joined(separator: ", ")
    }
}

/// Three lamps lit like a level meter while playing; a pause bar when
/// paused, so the shape tells the two apart as well as the colour.
private struct PlayingIndicator: View {
    let isPlaying: Bool
    @ScaledMetric(relativeTo: .caption) private var lamp = 4

    var body: some View {
        Group {
            if isPlaying {
                HStack(alignment: .bottom, spacing: lamp * 0.6) {
                    ForEach([2, 3, 1], id: \.self) { height in
                        VStack(spacing: lamp * 0.6) {
                            ForEach(0..<height, id: \.self) { _ in
                                Circle().fill(.lamp).frame(width: lamp, height: lamp)
                            }
                        }
                    }
                }
            } else {
                Image(systemName: "pause.fill")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(minWidth: 16)
        .accessibilityHidden(true)
    }
}

#Preview("Playing") {
    @Previewable @State var model = NowPlayingModel(client: .demo)
    PreviewHost(.demo) {
        TabView {
            Tab("Overview", systemImage: "gauge.with.dots.needle.33percent") { Color.clear }
        }
        .nowPlayingAccessory(model)
        .task { await model.poll() }
    }
}
