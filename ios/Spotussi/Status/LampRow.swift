import SpotussiKit
import SwiftUI

/// Progress as a row of lamps, like the web's catalog strip: lit for done,
/// dim for provisional, unlit for the rest.
struct LampRow: View {
    let lit: Int
    var dim = 0
    var count = 24
    @ScaledMetric(relativeTo: .caption) private var lampSize = 7

    /// Catalog lookups: lit for looked up, dim for provisional details.
    init(progress: CatalogProgress, count: Int = 24) {
        let lamps = progress.lamps(count: count)
        lit = lamps.lit
        dim = lamps.dim
        self.count = count
    }

    /// A fraction done, lit from the left.
    init(lit: Int, count: Int = 24) {
        self.lit = lit
        self.count = count
    }

    var body: some View {
        let lit = lit
        let dim = dim
        let count = count
        let lampSize = lampSize
        Canvas { context, size in
            let pitch = size.width / CGFloat(count)
            let side = min(lampSize, pitch * 0.75)
            for index in 0..<count {
                let rect = CGRect(
                    x: CGFloat(index) * pitch + (pitch - side) / 2,
                    y: (size.height - side) / 2, width: side, height: side)
                let lamp = Path(ellipseIn: rect)
                if index < lit {
                    context.fill(lamp, with: .style(Tone.lamp))
                } else if index < lit + dim {
                    var dimmed = context
                    dimmed.opacity = 0.38
                    dimmed.fill(lamp, with: .style(Tone.lamp))
                } else {
                    context.fill(lamp, with: .style(Tone.grille))
                }
            }
        }
        .frame(height: lampSize)
        .accessibilityHidden(true)
    }
}
