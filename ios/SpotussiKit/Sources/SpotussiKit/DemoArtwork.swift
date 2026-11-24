import CoreGraphics
import Foundation

/// Cover art for the demo, drawn on the phone. The demo's `image_url`s point
/// at the mock server's SVGs, which aren't bundled, so every artist, album
/// and playlist gets a tile drawn from its address instead: the same address
/// always draws the same tile, and no two neighbours look alike.
///
/// Abstract shapes in muted colours, so the sample never passes for a real
/// release.
public enum DemoArtwork {
    /// The kind of thing an address is for, from its path
    /// (`/_mock/img/<kind>/<id>.svg`).
    enum Kind: Equatable {
        case artist, album, playlist, other
    }

    /// The tile for `imageURL`, `maxPixelSize` pixels square; nil for a size
    /// of zero or less.
    @concurrent
    public static func image(for imageURL: String, maxPixelSize: Int) async -> CGImage? {
        draw(imageURL, side: maxPixelSize)
    }

    static func draw(_ imageURL: String, side: Int) -> CGImage? {
        guard side > 0,
            let space = CGColorSpace(name: CGColorSpace.sRGB),
            let context = CGContext(
                data: nil, width: side, height: side, bitsPerComponent: 8, bytesPerRow: 0,
                space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
        else { return nil }
        var random = Seeded(imageURL)
        let palette = palettes[random.next(below: palettes.count)]
        let size = CGFloat(side)

        // The field: the palette's base, darker towards one corner.
        let angle = CGFloat(random.next(below: 360)) * .pi / 180
        let from = CGPoint(x: size / 2 - cos(angle) * size / 2, y: size / 2 - sin(angle) * size / 2)
        let to = CGPoint(x: size / 2 + cos(angle) * size / 2, y: size / 2 + sin(angle) * size / 2)
        context.drawLinearGradient(
            gradient([palette.base, palette.deep], in: space), start: from, end: to,
            options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])

        switch kind(of: imageURL) {
        case .artist: glow(context, size: size, palette: palette, random: &random, space: space)
        case .playlist: stripes(context, size: size, palette: palette, random: &random)
        case .album, .other:
            switch random.next(below: 4) {
            case 0: rings(context, size: size, palette: palette, random: &random)
            case 1: sun(context, size: size, palette: palette, random: &random)
            case 2: meter(context, size: size, palette: palette, random: &random)
            default: stripes(context, size: size, palette: palette, random: &random)
            }
        }
        return context.makeImage()
    }

    static func kind(of imageURL: String) -> Kind {
        let parts = URL(string: imageURL)?.pathComponents ?? []
        guard parts.count >= 2 else { return .other }
        switch parts[parts.count - 2] {
        case "artist": return .artist
        case "album": return .album
        case "playlist": return .playlist
        default: return .other
        }
    }

    // MARK: - Motifs

    /// A soft light off-centre, for an artist's round tile.
    private static func glow(
        _ context: CGContext, size: CGFloat, palette: Palette, random: inout Seeded, space: CGColorSpace
    ) {
        let centre = CGPoint(
            x: size * (0.3 + 0.4 * random.unit()), y: size * (0.35 + 0.35 * random.unit()))
        context.drawRadialGradient(
            gradient([palette.accent.withAlpha(0.85), palette.accent.withAlpha(0)], in: space),
            startCenter: centre, startRadius: 0, endCenter: centre, endRadius: size * 0.75,
            options: [])
        let ring = size * (0.18 + 0.1 * random.unit())
        context.setStrokeColor(palette.light.withAlpha(0.35))
        context.setLineWidth(size * 0.012)
        context.strokeEllipse(
            in: CGRect(x: centre.x - ring, y: centre.y - ring, width: ring * 2, height: ring * 2))
    }

    /// Concentric rings, like a record's grooves.
    private static func rings(_ context: CGContext, size: CGFloat, palette: Palette, random: inout Seeded) {
        let centre = CGPoint(x: size * (0.25 + 0.5 * random.unit()), y: size * (0.25 + 0.5 * random.unit()))
        let count = 5 + random.next(below: 5)
        let step = size * 0.11
        context.setLineWidth(size * 0.022)
        for index in 0..<count {
            let radius = step * CGFloat(index + 1)
            let colour = index.isMultiple(of: 2) ? palette.accent : palette.light
            context.setStrokeColor(colour.withAlpha(0.75 - CGFloat(index) * 0.06))
            context.strokeEllipse(
                in: CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2))
        }
    }

    /// One large disc low on the tile, with a horizon line through it.
    private static func sun(_ context: CGContext, size: CGFloat, palette: Palette, random: inout Seeded) {
        let radius = size * (0.24 + 0.12 * random.unit())
        let centre = CGPoint(x: size * (0.3 + 0.4 * random.unit()), y: size * (0.38 + 0.2 * random.unit()))
        context.setFillColor(palette.accent)
        context.fillEllipse(
            in: CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2))
        context.setFillColor(palette.deep.withAlpha(0.9))
        let bands = 3 + random.next(below: 3)
        for band in 0..<bands {
            let height = size * 0.018 * CGFloat(band + 1)
            let y = centre.y - radius * 0.15 - CGFloat(band) * size * 0.07
            context.fill(CGRect(x: 0, y: y, width: size, height: height))
        }
    }

    /// Vertical bars of different heights, the brand's meter.
    private static func meter(_ context: CGContext, size: CGFloat, palette: Palette, random: inout Seeded) {
        let count = 6 + random.next(below: 5)
        let gap = size * 0.03
        let width = (size * 0.8 - gap * CGFloat(count - 1)) / CGFloat(count)
        for index in 0..<count {
            let height = size * (0.12 + 0.6 * random.unit())
            let x = size * 0.1 + CGFloat(index) * (width + gap)
            let colour = random.next(below: 3) == 0 ? palette.light : palette.accent
            context.setFillColor(colour.withAlpha(0.9))
            context.addPath(
                CGPath(
                    roundedRect: CGRect(x: x, y: size * 0.12, width: width, height: height),
                    cornerWidth: width / 2, cornerHeight: width / 2, transform: nil))
            context.fillPath()
        }
    }

    /// Diagonal bands across the tile.
    private static func stripes(_ context: CGContext, size: CGFloat, palette: Palette, random: inout Seeded) {
        let count = 3 + random.next(below: 4)
        let band = size / CGFloat(count * 2)
        let rising = random.next(below: 2) == 0
        context.saveGState()
        context.translateBy(x: size / 2, y: size / 2)
        context.rotate(by: (rising ? 1 : -1) * .pi / 4)
        for index in 0..<(count * 2) where index.isMultiple(of: 2) {
            let colour = index.isMultiple(of: 4) ? palette.accent : palette.light
            context.setFillColor(colour.withAlpha(0.7))
            context.fill(
                CGRect(x: -size, y: -size * 0.75 + CGFloat(index) * band * 1.5, width: size * 2, height: band))
        }
        context.restoreGState()
    }

    // MARK: - Colour

    struct Palette {
        let base: CGColor
        let deep: CGColor
        let accent: CGColor
        let light: CGColor
    }

    /// Muted pairs that sit on the app's graphite without fighting its green.
    static let palettes: [Palette] = [
        (0x1F3A44, 0x0E1A1F, 0x5FB3A8, 0xD8E6E3),  // teal
        (0x3B2340, 0x190E1C, 0xD27A97, 0xF1DCE4),  // plum
        (0x1D2A4A, 0x0C1222, 0x6E9BE0, 0xDCE5F5),  // navy
        (0x3A3A1E, 0x1A1A0C, 0xC7C25A, 0xEDECD2),  // olive
        (0x4A2A1C, 0x20110A, 0xE08B55, 0xF5E1D3),  // rust
        (0x2E2C44, 0x13121F, 0x9C93D8, 0xE4E2F3),  // slate
        (0x203826, 0x0D1A10, 0x7CC48A, 0xDCEEDF),  // forest
        (0x44202A, 0x1F0C11, 0xE2706F, 0xF6DADA),  // wine
        (0x2B3138, 0x111417, 0xB8C0C8, 0xF0F2F4),  // steel
        (0x423418, 0x1D160A, 0xE5B14E, 0xF7EBD2),  // amber
    ].map { Palette(base: rgb($0.0), deep: rgb($0.1), accent: rgb($0.2), light: rgb($0.3)) }

    private static func rgb(_ hex: UInt32) -> CGColor {
        CGColor(
            srgbRed: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255, alpha: 1)
    }

    private static func gradient(_ colours: [CGColor], in space: CGColorSpace) -> CGGradient {
        CGGradient(colorsSpace: space, colors: colours as CFArray, locations: nil)!
    }
}

extension CGColor {
    fileprivate func withAlpha(_ alpha: CGFloat) -> CGColor {
        copy(alpha: max(0, min(1, alpha))) ?? self
    }
}

/// A small generator seeded from a string (FNV-1a, then SplitMix64), so a
/// tile draws the same on every launch and every device.
struct Seeded {
    private var state: UInt64

    init(_ text: String) {
        var hash: UInt64 = 0xcbf2_9ce4_8422_2325
        for byte in text.utf8 {
            hash ^= UInt64(byte)
            hash &*= 0x100_0000_01b3
        }
        state = hash
    }

    mutating func next() -> UInt64 {
        state &+= 0x9e37_79b9_7f4a_7c15
        var z = state
        z = (z ^ (z >> 30)) &* 0xbf58_476d_1ce4_e5b9
        z = (z ^ (z >> 27)) &* 0x94d0_49bb_1331_11eb
        return z ^ (z >> 31)
    }

    mutating func next(below bound: Int) -> Int {
        Int(next() % UInt64(bound))
    }

    /// A number in 0..<1.
    mutating func unit() -> CGFloat {
        CGFloat(next() >> 11) / CGFloat(1 << 53)
    }
}
