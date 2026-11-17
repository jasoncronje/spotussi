import SpotussiKit
import SwiftUI

/// One play: artwork, track and artists, when it was played, and whether
/// it was skipped, too short to count, or of estimated length. Not a
/// button: whoever lists it decides where it leads.
struct PlayRow: View {
    /// How the time reads: "3 min ago" in a list of recent plays, the
    /// clock time in a log grouped by day, where a lit lamp also marks a
    /// counted play.
    enum Style {
        case recent, clock
    }

    let play: RecentPlay
    let now: Date
    var style = Style.recent
    /// Where the track's page zooms from, if it does.
    var zoom: ZoomOrigin?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        layout {
            ArtworkImage(url: play.imageURL, side: 40, symbol: "music.note")
                .zoomSource(zoom)
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
                Text(Self.time(play, now: now, style: style))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                if let note = play.note {
                    noteText(note)
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.tertiary)
                } else if style == .clock, play.isCounted {
                    Circle()
                        .fill(.lamp)
                        .frame(width: 6, height: 6)
                        .padding(.vertical, 3)
                }
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(play))
        .accessibilityValue(Self.spokenValue(play, now: now, style: style))
    }

    private static func time(_ play: RecentPlay, now: Date, style: Style) -> String {
        switch style {
        case .recent: Figures.playTime(play.playedAt, now: now)
        case .clock: Figures.clockTime(play.playedAt)
        }
    }

    private func noteText(_ note: RecentPlay.Note) -> Text {
        switch note {
        case .skipped(let at): Text("skipped at \(Figures.trackLength(milliseconds: at))")
        case .notCounted: Text("not counted")
        case .estimated(let length): Text("≈ \(Figures.trackLength(milliseconds: length))")
        }
    }

    /// "The Honest Paradise, Iris Sakoti"
    static func spokenLabel(_ play: RecentPlay) -> String {
        [play.name, play.artists].compactMap(\.self).joined(separator: ", ")
    }

    /// "8:19 AM, skipped at 0:22"
    static func spokenValue(_ play: RecentPlay, now: Date, style: Style = .recent) -> String {
        let when = time(play, now: now, style: style)
        guard let note = play.note else { return when }
        let detail =
            switch note {
            case .skipped(let at): String(localized: "skipped at \(Figures.trackLength(milliseconds: at))")
            case .notCounted: String(localized: "not counted")
            case .estimated(let length):
                String(localized: "about \(Figures.trackLength(milliseconds: length))")
            }
        return "\(when), \(detail)"
    }
}
