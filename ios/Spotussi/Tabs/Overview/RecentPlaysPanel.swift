import SpotussiKit
import SwiftUI

/// The latest plays, newest first: the Overview's, whatever the range, and
/// a track page's in its range. With a namespace, each play opens its
/// track's page.
struct RecentPlaysPanel: View {
    let title: LocalizedStringKey
    let plays: [RecentPlay]?
    let failed: Bool
    /// Nil where the plays are of the page's own track.
    var namespace: Namespace.ID?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            PanelTitle(title)
            if let plays {
                if plays.isEmpty {
                    Text("No plays yet.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                } else {
                    rows(plays)
                }
            } else if failed {
                SectionFailure()
            } else {
                rows(Self.placeholders)
                    .placeholder()
            }
        }
        .panel()
    }

    /// Times move on by the minute: "just now" becomes "3 min ago".
    private func rows(_ plays: [RecentPlay]) -> some View {
        TimelineView(.everyMinute) { context in
            VStack(spacing: 2) {
                ForEach(plays) { play in
                    row(play, now: context.date)
                }
            }
        }
    }

    @ViewBuilder private func row(_ play: RecentPlay, now: Date) -> some View {
        if let namespace, !play.trackID.isEmpty {
            let zoom = ZoomOrigin(id: "play:\(play.id)", namespace: namespace)
            RouteLink(
                route: .entity(EntityLink(kind: .track, id: play.trackID, name: play.name), zoom: zoom),
                accessibilityLabel: PlayRow.spokenLabel(play), accessibilityValue: PlayRow.spokenValue(play, now: now)
            ) {
                PlayRow(play: play, now: now, zoom: zoom)
            }
        } else {
            PlayRow(play: play, now: now)
        }
    }

    private static let placeholders: [RecentPlay] = (0..<5).map { index in
        RecentPlay(
            id: "placeholder-\(index)", trackID: "", name: "Placeholder track",
            artists: "Placeholder artist", imageURL: nil, playedAt: .now, note: nil)
    }
}
