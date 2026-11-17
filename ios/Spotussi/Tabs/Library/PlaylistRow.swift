import SpotussiKit
import SwiftUI

/// One playlist: artwork, name, whose it is and its size, how much of it
/// was played, and the plays started from it. Not a button.
struct PlaylistRow: View {
    let playlist: SavedPlaylist
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        let now = Date.now
        layout {
            ArtworkImage(url: playlist.imageURL, side: 40, symbol: "music.note.list")
            VStack(alignment: .leading, spacing: 2) {
                Text(playlist.name)
                    .font(.body.weight(.medium))
                Text(Self.about(playlist))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                if let playThrough = playlist.playThrough {
                    PlayThroughLine(playThrough: playThrough, text: Self.played(playThrough))
                        .padding(.top, 3)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: 2) {
                Text("\(playlist.playsHere) plays")
                    .font(.subheadline.weight(.semibold).monospacedDigit())
                if let last = playlist.lastPlayedHereAt {
                    Text("Last \(Figures.date(last, now: now))")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([playlist.name, Self.about(playlist)].joined(separator: ", "))
        .accessibilityValue(Self.spokenValue(playlist, now: now))
    }

    /// "Yours", "Collaborative · Jo", "Followed · Spotify"
    static func owner(_ playlist: SavedPlaylist) -> String {
        let relation =
            switch playlist.relation {
            case .own: String(localized: "Yours")
            case .collaborative: String(localized: "Collaborative")
            case .followed: String(localized: "Followed")
            }
        guard playlist.relation != .own, let name = playlist.ownerName else { return relation }
        return "\(relation) · \(name)"
    }

    /// "Yours · 45 songs"
    static func about(_ playlist: SavedPlaylist) -> String {
        guard let total = playlist.itemsTotal else { return owner(playlist) }
        return "\(owner(playlist)) · \(String(localized: "\(total) songs"))"
    }

    /// "96% played"
    static func played(_ playThrough: PlayThrough) -> String {
        String(localized: "\(Figures.percent(playThrough.share)) played")
    }

    /// "232 plays from it, last Oct 30, 96% played"
    static func spokenValue(_ playlist: SavedPlaylist, now: Date) -> String {
        var parts = [String(localized: "\(playlist.playsHere) plays from it")]
        if let last = playlist.lastPlayedHereAt {
            parts.append(String(localized: "last \(Figures.date(last, now: now))"))
        }
        if let playThrough = playlist.playThrough {
            parts.append(played(playThrough))
        }
        return parts.joined(separator: ", ")
    }
}
