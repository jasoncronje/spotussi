import SwiftUI

/// The brand's colours (D34, D40): graphite and aluminium grounds and one
/// highlight colour. Each is a pair, picked by the colour scheme, so views
/// write `.foregroundStyle(.lamp)` and get the right one in light and dark.
public struct Tone: ShapeStyle, Hashable, Sendable {
    let light: Color
    let dark: Color

    public func resolve(in environment: EnvironmentValues) -> Color {
        environment.colorScheme == .dark ? dark : light
    }

    init(light: UInt32, dark: UInt32) {
        self.light = Color(hex: light)
        self.dark = Color(hex: dark)
    }
}

extension ShapeStyle where Self == Tone {
    /// Lit lamps and single-series data: Spotify Green on graphite, the
    /// darker leaf on aluminium (3.1:1 there, the floor for graphics).
    public static var lamp: Tone { Tone(light: 0x169C45, dark: 0x1DB954) }
    /// Green text, dark enough to read on its ground.
    public static var lampInk: Tone { Tone(light: 0x107A35, dark: 0x1ED760) }
    /// Unlit lamps and chart grilles.
    public static var grille: Tone { Tone(light: 0xCFD1D4, dark: 0x3A3D46) }
    /// The page.
    public static var ground: Tone { Tone(light: 0xEDEEF0, dark: 0x121317) }
    /// Cards and rows on the page.
    public static var panel: Tone { Tone(light: 0xFFFFFF, dark: 0x1A1C21) }
}

extension Color {
    init(hex: UInt32) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255)
    }
}

extension Font {
    /// Big numbers: SF Pro at its expanded width, with digits that don't
    /// shift as they change. Scales with the text style it is given.
    public static func readout(_ style: Font.TextStyle = .largeTitle) -> Font {
        .system(style, weight: .semibold).width(.expanded).monospacedDigit()
    }

    /// Small capitals-style labels above a readout.
    public static var readoutLabel: Font {
        .system(.caption, weight: .medium).width(.expanded)
    }
}
