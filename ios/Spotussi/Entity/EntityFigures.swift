import SpotussiKit
import SwiftUI

/// Rank in the range, totals in the range and all time, and the first and
/// last play, as the web's readouts. The measure leads each total and the
/// other follows in the line under it. Without a detail it shows dashes,
/// for the placeholder.
struct EntityFigures: View {
    let kind: EntityKind
    let detail: EntityDetail?
    let measure: Measure
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        LazyVGrid(columns: columns, spacing: 12) {
            rank
            total(label: "In range", detail?.inRange)
            total(label: "All time", detail?.allTime)
            played
        }
    }

    private var rank: some View {
        guard let detail else {
            return Readout(label: "Rank", value: "#1", detail: " ")
        }
        guard let rank = detail.rank else {
            return Readout(label: "Rank", value: "–", spokenValue: String(localized: "None"), detail: String(localized: "Not played"))
        }
        return Readout(
            label: "Rank", value: "#\(Figures.count(rank))", spokenValue: Figures.count(rank),
            detail: outOf(detail.rankedTotal))
    }

    private func total(label: LocalizedStringKey, _ tally: EntityDetail.Tally?) -> some View {
        let value = tally?.value(for: measure) ?? 0
        return Readout(
            label: label, value: figure(value), spokenValue: Spoken.value(value, measure: measure),
            detail: tally.map(otherLine) ?? " ")
    }

    private var played: some View {
        let first = detail?.firstPlayed
        return Readout(
            label: "First played",
            // Month and year: a full date is clipped in the card's wide figures.
            value: first.map { $0.formatted(.dateTime.month(.abbreviated).year()) } ?? "–",
            detail: detail.map { detail -> String in
                guard let last = detail.lastPlayed else { return String(localized: "Never played") }
                return String(localized: "Last: \(RelativeTime.ago(last, now: .now))")
            } ?? " ")
    }

    private func figure(_ value: Int) -> String {
        measure == .plays ? Figures.count(value) : Figures.duration(milliseconds: value)
    }

    /// The other measure and the skips: "1h 7m · 8 skipped".
    private func otherLine(_ tally: EntityDetail.Tally) -> String {
        let other =
            measure == .plays ? Figures.duration(milliseconds: tally.milliseconds) : Spoken.plays(tally.plays)
        return "\(other) · \(String(localized: "\(tally.skips) skipped"))"
    }

    /// "of 568 artists"
    private func outOf(_ total: Int) -> String {
        switch kind {
        case .artist: String(localized: "of \(total) artists")
        case .album: String(localized: "of \(total) albums")
        case .track: String(localized: "of \(total) tracks")
        case .genre: String(localized: "of \(total) genres")
        }
    }

    private var columns: [GridItem] {
        let count = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        return Array(repeating: GridItem(.flexible(), spacing: 12, alignment: .top), count: count)
    }
}
