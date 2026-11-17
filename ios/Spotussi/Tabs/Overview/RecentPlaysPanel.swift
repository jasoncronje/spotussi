import SpotussiKit
import SwiftUI

/// The latest plays, newest first, whatever the range.
struct RecentPlaysPanel: View {
    let plays: [RecentPlay]?
    let failed: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            PanelTitle("Recently played")
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
                    .redacted(reason: .placeholder)
            }
        }
        .panel()
    }

    /// Times move on by the minute: "just now" becomes "3 min ago".
    private func rows(_ plays: [RecentPlay]) -> some View {
        TimelineView(.everyMinute) { context in
            VStack(spacing: 2) {
                ForEach(plays) { play in
                    PlayRow(play: play, now: context.date)
                }
            }
        }
    }

    private static let placeholders: [RecentPlay] = (0..<5).map { index in
        RecentPlay(
            id: "placeholder-\(index)", trackID: "", name: "Placeholder track",
            artists: "Placeholder artist", imageURL: nil, playedAt: .now, note: nil)
    }
}
