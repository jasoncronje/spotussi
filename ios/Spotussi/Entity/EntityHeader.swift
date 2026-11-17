import SpotussiKit
import SwiftUI

/// The head of an entity page: the artwork, running up under the bar, with
/// what the entity is and its name over it; then its links (an artist's
/// genres, an album's or track's artists and album) and details. A genre
/// has lamps instead of artwork.
struct EntityHeader: View {
    let link: EntityLink
    let name: String
    let detail: EntityDetail?

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if link.kind == .genre {
                genreHead
            } else {
                hero
            }
            facts
                .padding(.horizontal, 16)
        }
    }

    // MARK: - Head

    private var hero: some View {
        title
            .foregroundStyle(.white)
            .padding(.horizontal, 16)
            .padding(.bottom, 20)
            .padding(.top, 64)
            .frame(maxWidth: .infinity, minHeight: 320, alignment: .bottomLeading)
            .background {
                HeroArtwork(url: detail?.imageURL, symbol: link.kind.symbol)
                    .clipped()
                    .backgroundExtensionEffect()
                    .overlay {
                        // Keeps the name readable on any artwork.
                        LinearGradient(
                            colors: [.black.opacity(0), .black.opacity(0.62)],
                            startPoint: UnitPoint(x: 0.5, y: 0.3), endPoint: .bottom)
                    }
            }
    }

    private var genreHead: some View {
        HStack(alignment: .center, spacing: 16) {
            GenreGlyph(share: genreShare, side: 72, tile: .panel)
            title
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
    }

    private var title: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(kicker)
                .font(.caption.weight(.semibold).width(.expanded))
                .textCase(.uppercase)
                .opacity(0.8)
            Text(name)
                .font(.system(.largeTitle, weight: .bold).width(.expanded))
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }

    /// A genre's lamps, lit by its rank among genres, as on the web.
    private var genreShare: Double {
        guard let detail, let rank = detail.rank else { return 0.1 }
        return 1 - Double(rank - 1) / Double(max(1, detail.rankedTotal))
    }

    private var kicker: LocalizedStringKey {
        switch detail?.kicker {
        case .single: "Single"
        case .compilation: "Compilation"
        case .album: "Album"
        case .artist: "Artist"
        case .track: "Track"
        case .genre: "Genre"
        case nil:
            switch link.kind {
            case .artist: "Artist"
            case .album: "Album"
            case .track: "Track"
            case .genre: "Genre"
            }
        }
    }

    // MARK: - Links and details

    @ViewBuilder private var facts: some View {
        let links = links
        let details = details
        if !links.isEmpty || !details.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                if !links.isEmpty {
                    FlowLayout {
                        ForEach(links, id: \.self) { link in
                            EntityChip(link: link)
                        }
                    }
                }
                if !details.isEmpty {
                    Text(details.joined(separator: " · "))
                        .font(.subheadline.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
        } else if let detail, detail.kicker == .artist {
            Text("No genres yet.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
    }

    private var links: [EntityLink] {
        guard let detail else { return [] }
        switch detail.link.kind {
        case .artist: return detail.genres
        case .album: return detail.artists
        case .track: return detail.artists + [detail.album].compactMap(\.self)
        case .genre: return []
        }
    }

    /// "Dec 11, 2014 · 12 tracks", "3:14 · Explicit · Saved Oct 2, 2026",
    /// "37 artists"
    private var details: [String] {
        guard let detail else { return [] }
        var parts: [String] = []
        if let date = detail.releaseDate.flatMap({ Figures.releaseDate($0) }) {
            parts.append(date)
        }
        if let count = detail.trackCount {
            parts.append(String(localized: "\(count) tracks"))
        }
        if let duration = detail.duration {
            parts.append(Figures.trackLength(milliseconds: duration))
        }
        if detail.isExplicit {
            parts.append(String(localized: "Explicit"))
        }
        if let saved = detail.savedAt {
            parts.append(String(localized: "Saved \(saved.formatted(date: .abbreviated, time: .omitted))"))
        }
        if let count = detail.artistCount {
            parts.append(String(localized: "\(count) artists"))
        }
        return parts
    }
}

/// An artist, album or genre to open, as a solid chip.
struct EntityChip: View {
    let link: EntityLink

    var body: some View {
        NavigationLink(value: Route.entity(link)) {
            Label {
                Text(link.name)
            } icon: {
                Image(systemName: link.kind.symbol)
                    .foregroundStyle(.secondary)
            }
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, 14)
            .padding(.vertical, 6)
            .frame(minHeight: 44)
            .background(.panel, in: .capsule)
            .contentShape(.capsule)
        }
        .buttonStyle(.plain)
        .accessibilityValue(link.kind.name)
    }
}

extension EntityKind {
    /// "Artist", for VoiceOver after a name.
    var name: String {
        switch self {
        case .artist: String(localized: "Artist")
        case .album: String(localized: "Album")
        case .track: String(localized: "Track")
        case .genre: String(localized: "Genre")
        }
    }
}
