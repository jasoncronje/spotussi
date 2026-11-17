import SpotussiKit
import SwiftUI

/// Catalog progress as a row of lamps, like the web's catalog strip: lit
/// for looked up, dim for provisional details, unlit for the rest.
struct LampRow: View {
    let progress: CatalogProgress
    var count = 24
    @ScaledMetric(relativeTo: .caption) private var lampSize = 7

    var body: some View {
        let lamps = progress.lamps(count: count)
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
                if index < lamps.lit {
                    context.fill(lamp, with: .style(Tone.lamp))
                } else if index < lamps.lit + lamps.dim {
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
