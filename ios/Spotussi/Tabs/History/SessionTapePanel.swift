import SpotussiKit
import SwiftUI

/// Every row of a session at its real time between the first start and
/// the last end, as the tape on the web's session page: counted plays lit,
/// skipped or uncounted rows unlit, silence as gaps.
struct SessionTapePanel: View {
    let session: ListeningSession
    let tape: SessionTape

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            PanelTitle("Timeline")
            Canvas { context, size in
                for mark in tape.marks {
                    let rect = CGRect(
                        x: mark.start * size.width, y: 4, width: max(2, (mark.end - mark.start) * size.width - 1),
                        height: size.height - 8)
                    context.fill(
                        Path(roundedRect: rect, cornerRadius: 2), with: .style(mark.isLit ? Tone.lamp : Tone.grille))
                }
            }
            .frame(height: 32)
            .background(.ground, in: .rect(cornerRadius: 6))
            HStack {
                Text(Figures.clockTime(session.startedAt))
                Spacer(minLength: 8)
                Text(Figures.clockTime(session.endedAt))
            }
            .font(.caption2.monospacedDigit())
            .foregroundStyle(.secondary)
            FlowLayout(spacing: 14, lineSpacing: 4) {
                Key(tone: .lamp, text: String(localized: "Counted"))
                Key(tone: .grille, text: String(localized: "Skipped or short"))
                Text("Silence \(Figures.duration(milliseconds: session.silence))")
            }
            .font(.caption2)
            .foregroundStyle(.secondary)
        }
        .panel()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Timeline"))
        .accessibilityValue(spokenValue)
    }

    /// "30 counted, 4 skipped or short, silence 40 seconds"
    private var spokenValue: String {
        let lit = tape.marks.filter(\.isLit).count
        return [
            String(localized: "\(lit) counted"),
            String(localized: "\(tape.marks.count - lit) skipped or short"),
            String(localized: "silence \(Figures.duration(milliseconds: session.silence, width: .wide))"),
        ].joined(separator: ", ")
    }
}

/// A swatch and what it stands for.
private struct Key: View {
    let tone: Tone
    let text: String

    var body: some View {
        HStack(spacing: 5) {
            RoundedRectangle(cornerRadius: 2)
                .fill(tone)
                .frame(width: 10, height: 10)
            Text(text)
        }
    }
}
