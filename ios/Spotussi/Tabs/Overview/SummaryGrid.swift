import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Time listened, plays, tracks and artists, each with its change against
/// the period before. The measure decides whether plays or time leads.
/// Without a summary it shows zeros, for the placeholder.
struct SummaryGrid: View {
    let summary: Components.Schemas.OverviewSummary?
    let measure: Measure
    /// "vs the 4 weeks before"; nil for all time.
    let compareLabel: String?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            if let compareLabel, summary?.previousSummary != nil {
                Text(compareLabel)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            LazyVGrid(columns: columns, spacing: 12) {
                if measure == .plays {
                    plays
                    time
                } else {
                    time
                    plays
                }
                Readout(
                    label: "Tracks", value: Figures.count(current?.tracks ?? 0),
                    change: change(current?.tracks, previous?.tracks))
                Readout(
                    label: "Artists", value: Figures.count(current?.artists ?? 0),
                    change: change(current?.artists, previous?.artists))
            }
        }
    }

    private var time: some View {
        let milliseconds = current?.msPlayed ?? 0
        return Readout(
            label: "Time listened", value: Figures.duration(milliseconds: milliseconds),
            spokenValue: Figures.duration(milliseconds: milliseconds, width: .wide),
            change: change(current?.msPlayed, previous?.msPlayed))
    }

    private var plays: some View {
        Readout(
            label: "Plays", value: Figures.count(current?.plays ?? 0),
            change: change(current?.plays, previous?.plays))
    }

    private var current: Components.Schemas.Summary? { summary?.current }
    private var previous: Components.Schemas.Summary? { summary?.previousSummary }

    private func change(_ current: Int?, _ previous: Int?) -> Change? {
        guard let current else { return nil }
        return Change(current: current, previous: previous)
    }

    private var columns: [GridItem] {
        let count = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        return Array(repeating: GridItem(.flexible(), spacing: 12, alignment: .top), count: count)
    }
}
