import SpotussiKit
import SwiftUI

extension View {
    /// Content on a solid panel: cards, notices, readouts. Glass stays on
    /// the navigation layer.
    func panel(padding: CGFloat = 16) -> some View {
        self
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.panel, in: .rect(cornerRadius: 20))
    }
}

extension View {
    /// Placeholder content while the first answer loads: drawn redacted and
    /// hidden from VoiceOver, which would otherwise read made-up names and
    /// zeros.
    func placeholder(_ isPlaceholder: Bool = true) -> some View {
        self
            .redacted(reason: isPlaceholder ? .placeholder : [])
            .accessibilityHidden(isPlaceholder)
    }
}

extension ShapeStyle where Self == Color {
    /// Text and spinners on an accent-filled button: white on the light-mode
    /// green, near-black on the brighter dark-mode green, where white would
    /// fall below 3:1.
    static var accentLabel: Color { Color("OnAccent") }
}
