import SpotussiKit
import SwiftUI

/// One ranked entry: rank, artwork, name and artists, a meter for its share
/// of the first entry, and its figure with rank movement. Not a button:
/// whoever lists it decides where it leads.
struct TopRow: View {
    let entry: TopEntry
    let measure: Measure
    /// The entry's value against the first entry's, 0...1.
    let share: Double
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        // At the largest sizes the figure goes under the name, which then
        // has the row's width to wrap in.
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        layout {
            HStack(spacing: 12) {
                Text(entry.rank, format: .number)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .frame(minWidth: 18, alignment: .trailing)
                leading
            }
            VStack(alignment: .leading, spacing: 3) {
                Text(entry.name)
                    .font(.body.weight(.medium))
                if let detail = entry.detail {
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                ShareBar(share: share)
                    .frame(height: 3)
                    .padding(.top, 3)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: 3) {
                Text(entry.figure(for: measure))
                    .font(.subheadline.weight(.semibold).monospacedDigit())
                if let movement = entry.movement {
                    MovementMark(movement: movement)
                }
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityLabel)
        .accessibilityValue(accessibilityValue)
    }

    @ViewBuilder private var leading: some View {
        switch entry.kind {
        case .genre:
            GenreGlyph(share: share)
        case .artist:
            ArtworkImage(url: entry.imageURL, side: 40, symbol: "music.mic", isRound: true)
        case .track, .album:
            ArtworkImage(url: entry.imageURL, side: 40, symbol: "music.note")
        }
    }

    private var accessibilityLabel: String {
        let name = String(localized: "\(entry.rank). \(entry.name)")
        return [name, entry.detail].compactMap(\.self).joined(separator: ", ")
    }

    private var accessibilityValue: String {
        let figure = Spoken.value(entry.value(for: measure), measure: measure)
        guard let movement = entry.movement else { return figure }
        return "\(figure), \(MovementMark.spoken(movement))"
    }
}

/// Rank against the period before: "▲ 3", "▼ 2", "new", or "=".
struct MovementMark: View {
    let movement: Movement

    var body: some View {
        HStack(spacing: 2) {
            switch movement {
            case .new:
                Text("new")
                    .fontWeight(.semibold)
                    .foregroundStyle(.lampInk)
            case .same:
                Image(systemName: "equal")
            case .up(let places, _):
                Image(systemName: "arrowtriangle.up.fill")
                    .imageScale(.small)
                Text(places, format: .number)
            case .down(let places, _):
                Image(systemName: "arrowtriangle.down.fill")
                    .imageScale(.small)
                Text(places, format: .number)
            }
        }
        .font(.caption2.monospacedDigit())
        .foregroundStyle(.secondary)
        .accessibilityHidden(true)
    }

    static func spoken(_ movement: Movement) -> String {
        switch movement {
        case .new: String(localized: "new this period")
        case .same: String(localized: "same rank as before")
        case .up(let places, let from): String(localized: "up \(places) from rank \(from)")
        case .down(let places, let from): String(localized: "down \(places) from rank \(from)")
        }
    }
}

/// A thin meter: the lit part is the share.
private struct ShareBar: View {
    let share: Double

    var body: some View {
        Capsule()
            .fill(.grille)
            .overlay(alignment: .leading) {
                Capsule()
                    .fill(.lamp)
                    .scaleEffect(x: min(1, max(0.02, share)), anchor: .leading)
            }
            .accessibilityHidden(true)
    }
}

/// A genre has no artwork: a column of four lamps lit to its share.
private struct GenreGlyph: View {
    let share: Double
    @ScaledMetric(relativeTo: .body) private var side = 40

    var body: some View {
        let lit = max(1, Int((share * 4).rounded()))
        Canvas { context, size in
            let pitch = size.height / 5
            let lamp = pitch * 0.62
            for row in 0..<4 {
                let rect = CGRect(
                    x: (size.width - lamp) / 2, y: size.height - pitch * (CGFloat(row) + 1) - lamp / 2,
                    width: lamp, height: lamp)
                context.fill(Path(ellipseIn: rect), with: .style(row < lit ? Tone.lamp : Tone.grille))
            }
        }
        .frame(width: side, height: side)
        .background(.ground, in: .rect(cornerRadius: side * 0.12))
        .accessibilityHidden(true)
    }
}
