import SpotussiKit
import SwiftUI

/// A saved song's way into heavy rotation, as the web's rotation lamps:
/// one lamp per counted play up to the 5th, all of them haloed once it is
/// reached. A song played only as another release shows its first lamp as
/// a ring. Decorative: the plays figure beside it carries the value.
struct RotationLamps: View {
    let song: SavedSong
    @ScaledMetric(relativeTo: .caption) private var scaledSide: CGFloat = 7

    var body: some View {
        let count = SavedSong.rotationPlays
        let lit = song.litLamps(of: count)
        let isMaybe = song.status == .maybePlayed
        // Grows with the text, within reason: five lamps stay one short row.
        let side = min(scaledSide, 11)
        let pitch = side * 1.75
        Canvas { context, size in
            for index in 0..<count {
                let center = CGPoint(x: pitch * CGFloat(index) + pitch / 2, y: size.height / 2)
                let rect = CGRect(x: center.x - side / 2, y: center.y - side / 2, width: side, height: side)
                if index < lit {
                    if lit == count {
                        var halo = context
                        halo.opacity = 0.28
                        halo.fill(Path(ellipseIn: rect.insetBy(dx: -side * 0.36, dy: -side * 0.36)), with: .style(Tone.lamp))
                    }
                    context.fill(Path(ellipseIn: rect), with: .style(Tone.lamp))
                } else if isMaybe, index == 0 {
                    var ring = context
                    ring.opacity = 0.7
                    ring.stroke(Path(ellipseIn: rect.insetBy(dx: 0.75, dy: 0.75)), with: .style(Tone.lamp), lineWidth: 1.5)
                } else {
                    context.fill(Path(ellipseIn: rect), with: .style(Tone.grille))
                }
            }
        }
        .frame(width: pitch * CGFloat(count), height: side * 1.8)
        .accessibilityHidden(true)
    }
}

/// How many of an album's or playlist's songs were ever played, as the
/// web's share dots: one per song while they fit, else a share of the row,
/// the last lit one part-lit. Decorative: a figure beside it says it.
struct ShareDots: View {
    let playThrough: PlayThrough
    var limit = 24
    private static let side: CGFloat = 5

    var body: some View {
        let dots = playThrough.dots(limit: limit)
        let side = Self.side
        Canvas { context, size in
            let pitch = side * 2
            for index in 0..<dots.count {
                let rect = CGRect(x: pitch * CGFloat(index), y: (size.height - side) / 2, width: side, height: side)
                let lamp = Path(ellipseIn: rect)
                context.fill(lamp, with: .style(Tone.grille))
                let share = min(1, max(0, dots.lit - Double(index)))
                if share > 0 {
                    var lit = context
                    lit.opacity = share
                    lit.fill(lamp, with: .style(Tone.lamp))
                }
            }
        }
        .frame(width: side * 2 * CGFloat(dots.count) - side, height: side)
        .accessibilityHidden(true)
    }
}
