import SpotussiKit
import SwiftUI

/// One play: artwork, track and artists, when it was played, and whether
/// it was skipped, too short to count, or of estimated length. Not a
/// button: whoever lists it decides where it leads.
struct PlayRow: View {
    let play: RecentPlay
    let now: Date
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        layout {
            ArtworkImage(url: play.imageURL, side: 40, symbol: "music.note")
            VStack(alignment: .leading, spacing: 2) {
                Text(play.name)
                    .font(.body.weight(.medium))
                if let artists = play.artists {
                    Text(artists)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: 2) {
                Text(Figures.playTime(play.playedAt, now: now))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                if let note = play.note {
                    noteText(note)
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.tertiary)
                }
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([play.name, play.artists].compactMap(\.self).joined(separator: ", "))
        .accessibilityValue(accessibilityValue)
    }

    private func noteText(_ note: RecentPlay.Note) -> Text {
        switch note {
        case .skipped(let at): Text("skipped at \(Figures.trackLength(milliseconds: at))")
        case .notCounted: Text("not counted")
        case .estimated(let length): Text("≈ \(Figures.trackLength(milliseconds: length))")
        }
    }

    private var accessibilityValue: String {
        let time = Figures.playTime(play.playedAt, now: now)
        guard let note = play.note else { return time }
        let detail =
            switch note {
            case .skipped(let at): String(localized: "skipped at \(Figures.trackLength(milliseconds: at))")
            case .notCounted: String(localized: "not counted")
            case .estimated(let length):
                String(localized: "about \(Figures.trackLength(milliseconds: length))")
            }
        return "\(time), \(detail)"
    }
}
