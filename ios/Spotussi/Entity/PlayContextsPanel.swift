import SpotussiKit
import SwiftUI

/// Where the entity was played from: playlists, albums, artist pages and
/// Liked Songs, ranked by the measure, then what has no recorded source.
/// Read-only: naming Spotify's own playlists stays on the web (D17).
struct PlayContextsPanel: View {
    let contexts: PlayContexts?
    let measure: Measure
    let isFailed: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            PanelTitle("Played from")
            if isFailed {
                SectionFailure()
            } else if let contexts {
                if contexts.isEmpty(for: measure) {
                    Text("No plays in this range.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                } else {
                    rows(contexts)
                }
            } else {
                placeholders
            }
        }
        .panel()
    }

    private func rows(_ contexts: PlayContexts) -> some View {
        let leader = Double(contexts.leader(for: measure))
        return VStack(spacing: 2) {
            ForEach(contexts.items) { context in
                ContextRow(
                    rank: context.rank, symbol: symbol(context.kind), imageURL: context.imageURL,
                    name: name(context), kind: kindName(context), isUnresolved: context.name == nil,
                    value: context.value(for: measure), share: Double(context.value(for: measure)) / leader,
                    measure: measure)
            }
            let unknown = contexts.unknown(for: measure)
            if unknown > 0 {
                ContextRow(
                    rank: nil, symbol: "circle.dashed", imageURL: nil, name: String(localized: "Not recorded"),
                    kind: String(localized: "Imported history"), isUnresolved: true, value: unknown, share: nil,
                    measure: measure)
            }
        }
    }

    private var placeholders: some View {
        VStack(spacing: 2) {
            ForEach(1...3, id: \.self) { rank in
                ContextRow(
                    rank: rank, symbol: "music.note.list", imageURL: nil, name: "Placeholder name",
                    kind: "Playlist", isUnresolved: false, value: 0, share: 1, measure: measure)
            }
        }
        .redacted(reason: .placeholder)
    }

    /// The web's words for a context with no name.
    private func name(_ context: PlayContext) -> String {
        if let name = context.name { return name }
        switch context.kind {
        case .likedSongs: return String(localized: "Liked Songs")
        case .playlist where context.isSpotifyPlaylist: return String(localized: "Unnamed Spotify playlist")
        default: return String(localized: "Unknown")
        }
    }

    private func kindName(_ context: PlayContext) -> String? {
        let kind: String
        switch context.kind {
        case .likedSongs: return nil
        case .playlist: kind = String(localized: "Playlist")
        case .album: kind = String(localized: "Album")
        case .artist: kind = String(localized: "Artist page")
        case .other: kind = String(localized: "Other")
        }
        return context.isNamedByOwner ? String(localized: "\(kind), named by you") : kind
    }

    private func symbol(_ kind: PlayContext.Kind) -> String {
        switch kind {
        case .likedSongs: "heart"
        case .album: "square.stack"
        case .artist: "music.mic"
        case .playlist, .other: "music.note.list"
        }
    }
}

/// One source: rank, artwork or symbol, name and kind, a meter for its
/// share of the busiest, and its figure. One element for VoiceOver.
private struct ContextRow: View {
    let rank: Int?
    let symbol: String
    let imageURL: String?
    let name: String
    let kind: String?
    let isUnresolved: Bool
    let value: Int
    /// Against the busiest source; nil for the unrecorded rest.
    let share: Double?
    let measure: Measure
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .caption) private var rankWidth = 24

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        layout {
            HStack(spacing: 12) {
                Text(rank.map { Figures.count($0) } ?? "")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .frame(minWidth: rankWidth, alignment: .trailing)
                ArtworkImage(url: imageURL, side: 40, symbol: symbol)
            }
            VStack(alignment: .leading, spacing: 3) {
                Text(name)
                    .font(.body.weight(.medium))
                    .foregroundStyle(isUnresolved ? .secondary : .primary)
                if let kind {
                    Text(kind)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let share {
                    ShareBar(share: share)
                        .frame(height: 3)
                        .padding(.top, 3)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Text(measure == .plays ? Figures.count(value) : Figures.duration(milliseconds: value))
                .font(.subheadline.weight(.semibold).monospacedDigit())
                .foregroundStyle(share == nil ? .secondary : .primary)
                .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityValue(Spoken.value(value, measure: measure))
    }

    private var label: String {
        let named = rank.map { String(localized: "\($0). \(name)") } ?? name
        return [named, kind].compactMap(\.self).joined(separator: ", ")
    }
}
