import SpotussiKit
import SwiftUI

/// One saved album: artwork, name and artists, when it was saved, how many
/// of its tracks were played, and its plays and time. Not a button.
struct AlbumRow: View {
    let album: SavedAlbum
    var zoom: ZoomOrigin?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        let now = Date.now
        layout {
            ArtworkImage(url: album.imageURL, side: 40, symbol: "square.stack")
                .zoomSource(zoom)
            VStack(alignment: .leading, spacing: 2) {
                Text(album.album.name)
                    .font(.body.weight(.medium))
                if let artists = album.artists {
                    Text(artists)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let saved = Self.saved(album, now: now) {
                    Text(saved)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
                PlayThroughLine(playThrough: album.playThrough, text: Self.played(album))
                    .padding(.top, 3)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: 2) {
                Text("\(album.plays) plays")
                    .font(.subheadline.weight(.semibold).monospacedDigit())
                Text(Figures.duration(milliseconds: album.milliseconds))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([album.album.name, album.artists].compactMap(\.self).joined(separator: ", "))
        .accessibilityValue(Self.spokenValue(album, now: now))
    }

    static func saved(_ album: SavedAlbum, now: Date) -> String? {
        album.addedAt.map { String(localized: "Saved \(Figures.date($0, now: now))") }
    }

    /// "3 of 7 played", or "3 played" when the track count isn't known.
    static func played(_ album: SavedAlbum) -> String {
        guard let playThrough = album.playThrough else { return String(localized: "\(album.tracksPlayed) played") }
        return String(localized: "\(playThrough.played) of \(playThrough.total) played")
    }

    static func spokenValue(_ album: SavedAlbum, now: Date) -> String {
        [
            Spoken.plays(album.plays), Figures.duration(milliseconds: album.milliseconds, width: .wide),
            played(album), saved(album, now: now),
        ]
        .compactMap(\.self).joined(separator: ", ")
    }
}

/// Share dots and the figure they stand for: "3 of 7 played".
struct PlayThroughLine: View {
    let playThrough: PlayThrough?
    let text: String

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 8) {
                dots
                label
            }
            VStack(alignment: .leading, spacing: 4) {
                dots
                label
            }
        }
    }

    @ViewBuilder private var dots: some View {
        if let playThrough {
            ShareDots(playThrough: playThrough, limit: 12)
        }
    }

    private var label: some View {
        Text(text)
            .font(.caption.monospacedDigit())
            .foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
    }
}
