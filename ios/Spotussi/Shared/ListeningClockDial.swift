import SpotussiKit
import SwiftUI

/// The listening clock as the brand's lamps: 24 spokes, midnight at the
/// top, running clockwise, each lit from the centre outwards to its share
/// of the busiest hour, the outermost lit lamp in part. Tapping a spoke
/// picks its hour and dims the rest; tapping it again lets go. Without
/// hours it draws the empty sockets, as the placeholder.
struct ListeningClockDial: View {
    let hours: HourlyListening?
    @Binding var selected: Int?
    @State private var side: CGFloat = 0

    var body: some View {
        Canvas { context, size in
            draw(in: &context, size: size)
        }
        .aspectRatio(1, contentMode: .fit)
        .frame(maxWidth: 340)
        .onGeometryChange(for: CGFloat.self) { proxy in
            min(proxy.size.width, proxy.size.height)
        } action: { side in
            self.side = side
        }
        // The quarter labels are decoration (the readout says the hour), so
        // they stop growing before they would leave the dial.
        .dynamicTypeSize(...DynamicTypeSize.xLarge)
        .contentShape(.circle)
        .onTapGesture { location in
            guard let hour = hour(at: location) else { return }
            selected = selected == hour ? nil : hour
        }
        .accessibilityHidden(true)
    }

    // MARK: - Geometry

    private struct Metrics {
        let center: CGPoint
        let inner: CGFloat
        let outer: CGFloat
        let lamp: CGFloat

        init(side: CGFloat) {
            center = CGPoint(x: side / 2, y: side / 2)
            inner = side * 0.17
            outer = side * 0.34
            lamp = min(side * 0.0115, (outer - inner) / CGFloat(HourlyListening.rings - 1) * 0.36)
        }

        var step: CGFloat { (outer - inner) / CGFloat(HourlyListening.rings - 1) }

        func angle(_ hour: Double) -> Double { hour / 24 * 2 * .pi - .pi / 2 }

        func point(_ hour: Int, radius: CGFloat) -> CGPoint {
            let angle = angle(Double(hour))
            return CGPoint(x: center.x + cos(angle) * radius, y: center.y + sin(angle) * radius)
        }
    }

    /// The hour whose spoke is under `location`, if it is on the dial.
    private func hour(at location: CGPoint) -> Int? {
        guard side > 0 else { return nil }
        let metrics = Metrics(side: side)
        let dx = location.x - metrics.center.x
        let dy = location.y - metrics.center.y
        let distance = (dx * dx + dy * dy).squareRoot()
        guard distance > metrics.inner * 0.6, distance < metrics.outer + side * 0.1 else { return nil }
        // Clockwise from the top, in hours.
        var turns = (atan2(dy, dx) + .pi / 2) / (2 * .pi)
        if turns < 0 { turns += 1 }
        return Int((turns * 24).rounded()) % 24
    }

    // MARK: - Drawing

    private func draw(in context: inout GraphicsContext, size: CGSize) {
        let side = min(size.width, size.height)
        guard side > 0 else { return }
        let metrics = Metrics(side: side)
        for hour in 0..<24 {
            var spoke = context
            if let selected, selected != hour { spoke.opacity = 0.4 }
            let item = hours?.hours[hour]
            for ring in 0..<HourlyListening.rings {
                let point = metrics.point(hour, radius: metrics.inner + CGFloat(ring) * metrics.step)
                let lamp = Path(
                    ellipseIn: CGRect(
                        x: point.x - metrics.lamp, y: point.y - metrics.lamp, width: metrics.lamp * 2,
                        height: metrics.lamp * 2))
                spoke.fill(lamp, with: .style(Tone.grille))
                let light = item.flatMap { hours?.light($0, ring: ring) } ?? 0
                if light > 0 {
                    var lit = spoke
                    lit.opacity *= light
                    lit.fill(lamp, with: .style(Tone.lamp))
                }
            }
        }
        if let selected {
            let point = metrics.point(selected, radius: metrics.outer + metrics.step * 1.1)
            let mark = Path(ellipseIn: CGRect(x: point.x - 2.5, y: point.y - 2.5, width: 5, height: 5))
            context.fill(mark, with: .style(HierarchicalShapeStyle.primary))
        }
        for quarter in [0, 6, 12, 18] {
            let label = context.resolve(
                Text(Figures.hour(quarter)).font(.caption2.weight(.medium)).foregroundStyle(.secondary))
            context.draw(label, at: metrics.point(quarter, radius: metrics.outer + side * 0.09))
        }
    }
}
