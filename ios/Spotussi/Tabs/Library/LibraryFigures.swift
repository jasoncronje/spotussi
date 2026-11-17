import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The Library's readouts, as the web's: what is saved, what of it was
/// never played, and how long saving takes to turn into a first play and
/// into heavy rotation. Without a summary it shows zeros, for the
/// placeholder.
struct LibraryFigures: View {
    let summary: Components.Schemas.LibrarySummary?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        LazyVGrid(columns: columns, spacing: 12) {
            Readout(label: "Saved songs", value: Figures.count(summary?.savedTracks ?? 0), detail: saved)
            Readout(label: "Never played", value: Figures.count(neverPlayed), detail: neverPlayedShare)
            days(label: "To first play", summary?.daysToFirstPlay)
            days(label: "To rotation", summary?.daysToRotation)
        }
    }

    /// "158 albums · 48 playlists"
    private var saved: String {
        let albums = String(localized: "\(summary?.savedAlbums ?? 0) albums")
        let playlists = String(localized: "\(summary?.playlists ?? 0) playlists")
        return "\(albums) · \(playlists)"
    }

    private var neverPlayed: Int { summary?.tracks.neverPlayed ?? 0 }

    /// "14% of saved songs"
    private var neverPlayedShare: String {
        let total = summary?.savedTracks ?? 0
        let share = total > 0 ? Double(neverPlayed) / Double(total) : 0
        return String(localized: "\(Figures.percent(share)) of saved songs")
    }

    /// The median days from saving, and how many songs it is the median of:
    /// "266" over "days, median of 732".
    private func days(label: LocalizedStringKey, _ days: Components.Schemas.LibraryDays?) -> some View {
        let median = days?.median
        let count = days?.count ?? 0
        let detail =
            median == 1
            ? String(localized: "day, median of \(count)") : String(localized: "days, median of \(count)")
        return Readout(
            label: label, value: Figures.days(median),
            spokenValue: median == nil ? String(localized: "None yet") : nil, detail: detail)
    }

    private var columns: [GridItem] {
        let count = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        return Array(repeating: GridItem(.flexible(), spacing: 12, alignment: .top), count: count)
    }
}
