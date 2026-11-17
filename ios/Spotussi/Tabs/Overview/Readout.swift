import SpotussiKit
import SwiftUI

/// A labelled figure on a panel, with its change against the period before.
struct Readout: View {
    let label: LocalizedStringKey
    let value: String
    /// What VoiceOver reads for `value` when the short form doesn't speak
    /// well ("3h 12m").
    var spokenValue: String?
    /// Nil when there is nothing to compare with.
    var change: Change?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.readoutLabel)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.readout(.title))
                .contentTransition(.numericText())
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .accessibilityLabel(spokenValue ?? value)
            if let change {
                ChangeLine(change: change)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .panel()
        .accessibilityElement(children: .combine)
    }
}

/// "▲ 37%": the arrow's direction and the words say which way; the colour
/// only underlines a rise.
private struct ChangeLine: View {
    let change: Change

    var body: some View {
        HStack(spacing: 4) {
            switch change {
            case .new:
                Text("New")
            case .same:
                Image(systemName: "equal")
                    .imageScale(.small)
                Text(Change.Percent(value: 0, fractionDigits: 0).formatted())
            case .rise(let percent):
                Image(systemName: "arrowtriangle.up.fill")
                    .imageScale(.small)
                Text(percent.formatted())
            case .fall(let percent):
                Image(systemName: "arrowtriangle.down.fill")
                    .imageScale(.small)
                Text(percent.formatted())
            }
        }
        .font(.caption.weight(.semibold).monospacedDigit())
        .foregroundStyle(isLit ? AnyShapeStyle(.lampInk) : AnyShapeStyle(.secondary))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityText)
    }

    private var isLit: Bool {
        switch change {
        case .new, .rise: true
        case .same, .fall: false
        }
    }

    private var accessibilityText: Text {
        switch change {
        case .new: Text("New")
        case .same: Text("No change")
        case .rise(let percent): Text("Up \(percent.formatted())")
        case .fall(let percent): Text("Down \(percent.formatted())")
        }
    }
}
