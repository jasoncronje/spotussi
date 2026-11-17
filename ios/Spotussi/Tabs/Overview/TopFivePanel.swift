import SpotussiKit
import SwiftUI

/// The range's top five of one kind. Without entries it shows five
/// placeholder rows, for the first load.
struct TopFivePanel: View {
    let kind: TopEntry.Kind
    let entries: [TopEntry]?
    let measure: Measure
    let isFailed: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            PanelTitle(title)
            if isFailed {
                SectionFailure()
            } else if let entries, entries.isEmpty {
                (kind == .genre ? Text("No genres yet.") : Text("No plays in this range."))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                let rows = entries ?? placeholders
                let top = max(1, rows.map { $0.value(for: measure) }.max() ?? 1)
                VStack(spacing: 2) {
                    ForEach(rows) { entry in
                        TopRow(
                            entry: entry, measure: measure,
                            share: Double(entry.value(for: measure)) / Double(top))
                    }
                }
            }
        }
        .panel()
    }

    private var title: LocalizedStringKey {
        switch kind {
        case .artist: "Top artists"
        case .track: "Top tracks"
        case .album: "Top albums"
        case .genre: "Top genres"
        }
    }

    /// Rows shaped like real ones, redacted by the screen.
    private var placeholders: [TopEntry] {
        (1...5).map { rank in
            TopEntry(
                kind: kind, entityID: "placeholder-\(rank)", rank: rank, name: "Placeholder name",
                detail: kind == .track || kind == .album ? "Placeholder artist" : nil, imageURL: nil,
                plays: 10 - rank, milliseconds: 0, movement: nil)
        }
    }
}
