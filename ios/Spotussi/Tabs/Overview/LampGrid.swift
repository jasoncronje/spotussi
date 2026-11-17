import SpotussiKit
import SwiftUI

/// The listening series as the brand's lamp grid: a column of lamps per
/// bucket, lit from the bottom to its share of the busiest column. Dim lit
/// lamps are estimated time; unlit lamps above show where the period before
/// reached; the rest of the grid is empty sockets. Without a series it
/// draws the empty grid, as the placeholder.
struct LampGrid: View {
    let series: ListeningSeries?
    static let rows = 8
    /// Columns in the placeholder: four weeks of days.
    private static let placeholderColumns = 28

    var body: some View {
        let columns = series.map { series in
            series.columns.map { series.lamps($0, rows: Self.rows) }
        }
        Canvas { context, size in
            let count = columns?.count ?? Self.placeholderColumns
            guard count > 0 else { return }
            let pitchX = size.width / CGFloat(count)
            let pitchY = size.height / CGFloat(Self.rows)
            let side = min(pitchX * 0.72, pitchY * 0.62, 8)
            for index in 0..<count {
                let lamps = columns?[index]
                for row in 0..<Self.rows {
                    let rect = CGRect(
                        x: (CGFloat(index) + 0.5) * pitchX - side / 2,
                        y: size.height - (CGFloat(row) + 0.5) * pitchY - side / 2,
                        width: side, height: side)
                    Self.draw(Path(ellipseIn: rect), state: lamps.map { Self.state(row, of: $0) } ?? .socket, in: context)
                }
            }
        }
        .frame(height: 112)
        .accessibilityHidden(true)
    }

    private enum LampState {
        case lit, estimated, previous, socket
    }

    private static func state(_ row: Int, of lamps: ListeningSeries.Lamps) -> LampState {
        if row < lamps.lit { return .lit }
        if row < lamps.lit + lamps.estimated { return .estimated }
        if row < lamps.lit + lamps.estimated + lamps.previous { return .previous }
        return .socket
    }

    private static func draw(_ lamp: Path, state: LampState, in context: GraphicsContext) {
        var context = context
        switch state {
        case .lit:
            context.fill(lamp, with: .style(Tone.lamp))
        case .estimated:
            context.opacity = 0.38
            context.fill(lamp, with: .style(Tone.lamp))
        case .previous:
            context.fill(lamp, with: .style(Tone.grille))
        case .socket:
            context.opacity = 0.35
            context.fill(lamp, with: .style(Tone.grille))
        }
    }
}
