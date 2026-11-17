// Draws the app icon: the level mark (D19, brand/README.md) on the dark
// graphite plate, as 1024 x 1024 PNGs for the asset catalog.
//
//   swift ios/scripts/make-app-icon.swift
//
// Writes AppIcon.png (opaque, for the light and default appearance),
// AppIcon-Dark.png and AppIcon-Tinted.png (transparent grounds, which iOS
// fills itself) into ios/Spotussi/Assets.xcassets/AppIcon.appiconset.
//
// Colours and geometry are the app's own: Tone.panel, Tone.lamp and
// Tone.grille in Theme.swift (their dark values), and LevelMark.swift's
// 4 x 4 lamps lit to 2 · 4 · 3 · 1, 5.5 across on a pitch of 7.

import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let side = 1024
/// How much of the icon's width the mark spans. The rest is clear ground,
/// well inside the corner mask.
let markShare = 0.54
let heights = [2, 4, 3, 1]

struct Palette {
    let ground: UInt32?
    let lit: UInt32
    let unlit: UInt32
}

let palettes: [(file: String, palette: Palette)] = [
    // Graphite plate (Tone.panel), Spotify Green lamps (Tone.lamp) and the
    // grille (Tone.grille).
    ("AppIcon.png", Palette(ground: 0x1A1C21, lit: 0x1DB954, unlit: 0x3A3D46)),
    ("AppIcon-Dark.png", Palette(ground: nil, lit: 0x1DB954, unlit: 0x3A3D46)),
    // Greys only: iOS tints by brightness.
    ("AppIcon-Tinted.png", Palette(ground: nil, lit: 0xFFFFFF, unlit: 0x4A4A4A)),
]

func color(_ hex: UInt32, in space: CGColorSpace) -> CGColor {
    let components: [CGFloat] = [
        CGFloat((hex >> 16) & 0xFF) / 255, CGFloat((hex >> 8) & 0xFF) / 255, CGFloat(hex & 0xFF) / 255, 1,
    ]
    return CGColor(colorSpace: space, components: components)!
}

func draw(_ palette: Palette) -> CGImage {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    // The default icon has no alpha channel at all, as App Store Connect
    // requires; the others need one for their transparent ground.
    let alpha: CGImageAlphaInfo = palette.ground == nil ? .premultipliedLast : .noneSkipLast
    let context = CGContext(
        data: nil, width: side, height: side, bitsPerComponent: 8, bytesPerRow: 0, space: space,
        bitmapInfo: alpha.rawValue)!
    let full = CGRect(x: 0, y: 0, width: side, height: side)
    if let ground = palette.ground {
        context.setFillColor(color(ground, in: space))
        context.fill(full)
    } else {
        context.clear(full)
    }

    let span = Double(side) * markShare
    let pitch = span / (3 + 5.5 / 7)
    let lamp = pitch * 5.5 / 7
    let origin = (Double(side) - span) / 2
    for column in 0..<4 {
        // Rows count up from the bottom: CoreGraphics' y axis points up.
        for row in 0..<4 {
            let isLit = row < heights[column]
            context.setFillColor(color(isLit ? palette.lit : palette.unlit, in: space))
            context.fillEllipse(
                in: CGRect(
                    x: origin + Double(column) * pitch, y: origin + Double(row) * pitch, width: lamp,
                    height: lamp))
        }
    }
    return context.makeImage()!
}

func write(_ image: CGImage, to url: URL) {
    let destination = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else {
        fatalError("Couldn't write \(url.path)")
    }
}

let script = URL(fileURLWithPath: #filePath).standardizedFileURL
let iconSet = script.deletingLastPathComponent().deletingLastPathComponent()
    .appending(path: "Spotussi/Assets.xcassets/AppIcon.appiconset")
for (file, palette) in palettes {
    let url = iconSet.appending(path: file)
    write(draw(palette), to: url)
    print("wrote \(url.path)")
}
