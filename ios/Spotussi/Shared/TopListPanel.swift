import SpotussiKit
import SwiftUI

/// A short ranking on a panel: the Overview's top fives and the lists on
/// entity pages. Each row opens its page. Without entries it shows
/// placeholder rows, for the first load.
struct TopListPanel: View {
    let title: LocalizedStringKey
    let kind: EntityKind
    let entries: [TopEntry]?
    let measure: Measure
    let isFailed: Bool
    /// The screen's namespace, for zooming into a page from its row.
    let namespace: Namespace.ID
    var placeholderCount = 5
    /// The whole ranking, offered when it is longer than the panel.
    var all: AllLink?

    struct AllLink {
        let route: Route
        let total: Int
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline) {
                    PanelTitle(title)
                    Spacer(minLength: 12)
                    allLink
                }
                VStack(alignment: .leading, spacing: 2) {
                    PanelTitle(title)
                    allLink
                }
            }
            if isFailed {
                SectionFailure()
            } else if let entries {
                if entries.isEmpty {
                    (kind == .genre ? Text("No genres yet.") : Text("No plays in this range."))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                } else {
                    rows(entries)
                }
            } else {
                placeholders
            }
        }
        .panel()
    }

    @ViewBuilder private var allLink: some View {
        if let all, let entries, all.total > entries.count {
            NavigationLink(value: all.route) {
                HStack(spacing: 4) {
                    Text("All \(Figures.count(all.total))")
                    Image(systemName: "chevron.forward")
                        .imageScale(.small)
                        .accessibilityHidden(true)
                }
                .font(.subheadline.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(minHeight: 44)
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(Text(title))
            .accessibilityValue(Text("All \(Figures.count(all.total))"))
        }
    }

    private func rows(_ entries: [TopEntry]) -> some View {
        let top = max(1, entries.map { $0.value(for: measure) }.max() ?? 1)
        return VStack(spacing: 2) {
            ForEach(entries) { entry in
                TopEntryLink(
                    entry: entry, measure: measure, share: Double(entry.value(for: measure)) / Double(top),
                    namespace: namespace)
            }
        }
    }

    /// Rows shaped like real ones, redacted, and not links.
    private var placeholders: some View {
        VStack(spacing: 2) {
            ForEach(1...placeholderCount, id: \.self) { rank in
                TopRow(
                    entry: TopEntry.placeholder(kind: kind, rank: rank), measure: measure,
                    share: Double(placeholderCount + 1 - rank) / Double(placeholderCount))
            }
        }
        .redacted(reason: .placeholder)
    }
}

/// A ranked entry that opens its page, zooming out of its artwork.
struct TopEntryLink: View {
    let entry: TopEntry
    let measure: Measure
    let share: Double
    let namespace: Namespace.ID

    var body: some View {
        // A genre has lamps, not artwork, to zoom from.
        let zoom = entry.kind == .genre ? nil : ZoomOrigin(id: entry.id, namespace: namespace)
        RouteLink(
            route: .entity(entry.link, zoom: zoom), accessibilityLabel: TopRow.spokenLabel(entry),
            accessibilityValue: TopRow.spokenValue(entry, measure: measure), spotifyURI: entry.spotifyURI
        ) {
            TopRow(entry: entry, measure: measure, share: share, zoom: zoom)
        }
    }
}

extension TopEntry {
    /// A row shaped like a real one, for redacted placeholders.
    static func placeholder(kind: EntityKind, rank: Int) -> TopEntry {
        TopEntry(
            kind: kind, entityID: "placeholder-\(rank)", rank: rank, name: "Placeholder name",
            detail: kind == .track || kind == .album ? "Placeholder artist" : nil, imageURL: nil,
            plays: 0, milliseconds: 0, movement: nil)
    }
}
