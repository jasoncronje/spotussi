import SpotussiAPI
import SpotussiKit
import SwiftUI

/// One session: its rank, day and times, plays, a meter against the
/// longest, the artists played most, and its length. Not a button:
/// whoever lists it decides where it leads.
struct SessionRow: View {
    let rank: Int
    let session: ListeningSession
    /// Its length against the longest's, 0...1.
    let share: Double
    let now: Date
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .caption) private var rankWidth = 24
    /// Room for "1h 40m", so every row's meter ends at the same place.
    @ScaledMetric(relativeTo: .subheadline) private var lengthWidth = 56

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: 12))
        layout {
            Text(rank, format: .number)
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(minWidth: rankWidth, alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing)
            VStack(alignment: .leading, spacing: 3) {
                Text(Figures.day(session.startedAt, now: now))
                    .font(.body.weight(.medium))
                Text("\(Figures.timeSpan(session.startedAt, session.endedAt)) · \(session.plays) plays")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                ShareBar(share: share)
                    .frame(height: 3)
                    .padding(.vertical, 3)
                if let names = session.artistNames {
                    Text(names)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Text(Figures.duration(milliseconds: session.duration))
                .font(.subheadline.weight(.semibold).monospacedDigit())
                .fixedSize()
                .frame(
                    minWidth: dynamicTypeSize.isAccessibilitySize ? nil : lengthWidth,
                    alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing)
        }
        .padding(.vertical, 8)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(session, rank: rank, now: now))
        .accessibilityValue(Self.spokenValue(session))
    }

    /// "1. Wed, Oct 21, 2:46 – 4:26 PM"
    static func spokenLabel(_ session: ListeningSession, rank: Int, now: Date) -> String {
        let day = Figures.day(session.startedAt, now: now)
        return "\(String(localized: "\(rank). \(day)")), \(Figures.timeSpan(session.startedAt, session.endedAt))"
    }

    /// "1 hour, 40 minutes, 32 plays, 9 skipped, mostly Bitter Parade, DJ Elny"
    static func spokenValue(_ session: ListeningSession) -> String {
        var parts = [
            Figures.duration(milliseconds: session.duration, width: .wide), Spoken.plays(session.plays),
            String(localized: "\(session.skips) skipped"),
        ]
        if let names = session.artistNames {
            parts.append(String(localized: "mostly \(names)"))
        }
        return parts.joined(separator: ", ")
    }

    /// A row shaped like a real one, for redacted placeholders.
    static func placeholder(rank: Int) -> SessionRow {
        let start: Int64 = 1_792_939_587_000
        let session = Components.Schemas.Session(
            durationMs: 5_400_000, endedAt: start + 5_400_000, id: "placeholder-\(rank)", msPlayed: 5_000_000,
            plays: 30, rows: 32, skips: 2, startedAt: start,
            topArtists: [Components.Schemas.ArtistRef(id: "placeholder", name: "Placeholder artist")])
        return SessionRow(
            rank: rank, session: ListeningSession(session), share: 1 - Double(rank) / 8, now: .now)
    }
}
