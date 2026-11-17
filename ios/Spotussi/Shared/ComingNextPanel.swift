import SpotussiKit
import SwiftUI

/// Stands in for a screen that a later update fills: what will be here,
/// said plainly.
struct ComingNextPanel: View {
    let symbol: String
    let title: LocalizedStringKey
    let message: LocalizedStringKey

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Image(systemName: symbol)
                .font(.title2)
                .foregroundStyle(.lampInk)
                .accessibilityHidden(true)
            Text(title)
                .font(.headline)
            Text(message)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .panel(padding: 20)
        .accessibilityElement(children: .combine)
    }
}
