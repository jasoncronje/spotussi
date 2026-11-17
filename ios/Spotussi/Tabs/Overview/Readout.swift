import SpotussiKit
import SwiftUI

/// A labelled figure on a panel, with its change against the period before.
struct Readout: View {
    let label: LocalizedStringKey
    let value: String
    /// Whole percent against the period before; nil when there is nothing
    /// to compare with.
    var change: Int?

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
            if let change {
                ChangeLine(percent: change)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .panel()
        .accessibilityElement(children: .combine)
    }
}

/// "▲ 37%": the arrow's direction and the sign say which way, the colour
/// only underlines a rise.
private struct ChangeLine: View {
    let percent: Int

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: symbol)
                .imageScale(.small)
            Text(percent.formatted(.percent.sign(strategy: .always(includingZero: false))))
                .monospacedDigit()
        }
        .font(.caption.weight(.semibold))
        .foregroundStyle(percent > 0 ? AnyShapeStyle(.lampInk) : AnyShapeStyle(.secondary))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityText)
    }

    private var symbol: String {
        if percent > 0 { "arrowtriangle.up.fill" }
        else if percent < 0 { "arrowtriangle.down.fill" }
        else { "equal" }
    }

    private var accessibilityText: String {
        if percent > 0 { String(localized: "Up \(percent.formatted(.percent))") }
        else if percent < 0 { String(localized: "Down \(abs(percent).formatted(.percent))") }
        else { String(localized: "No change") }
    }
}
