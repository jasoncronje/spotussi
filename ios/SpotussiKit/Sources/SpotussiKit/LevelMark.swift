import SwiftUI

/// The mark: a 4 × 4 grid of lamps lit to heights 2 · 4 · 3 · 1.
public struct LevelMark: View {
    static let heights = [2, 4, 3, 1]

    public init() {}

    public var body: some View {
        Canvas { context, size in
            // Same geometry as brand/logo/mark.svg: lamps 5.5 across on a pitch of 7.
            let side = min(size.width, size.height)
            let pitch = side / (3 + 5.5 / 7)
            let lampSize = pitch * 5.5 / 7
            for column in 0..<4 {
                for row in 0..<4 {
                    let lit = (4 - row) <= Self.heights[column]
                    let rect = CGRect(
                        x: CGFloat(column) * pitch, y: CGFloat(row) * pitch,
                        width: lampSize, height: lampSize)
                    context.fill(
                        Path(ellipseIn: rect), with: .style(lit ? Tone.lamp : Tone.grille))
                }
            }
        }
        .aspectRatio(1, contentMode: .fit)
        .accessibilityHidden(true)
    }
}
