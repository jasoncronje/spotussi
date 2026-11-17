import SpotussiKit
import SwiftUI

/// When saved songs reached heavy rotation, as the web's mix meter: one
/// row of dots split into runs by part, from before saving to not yet,
/// and a legend with each part's share. The legend's words and order carry
/// the parts; the tones only group the dots.
struct RotationMixPanel: View {
    let mix: RotationMix?
    /// The counted play that marks heavy rotation.
    let rotationPlays: Int
    @Environment(\.colorScheme) private var scheme

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ChartHeader(title: String(localized: "Reached rotation")) {
                Text("At \(rotationPlays) plays")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            if let mix, mix.total > 0 {
                DotRow(tones: tones(mix))
                legend(mix)
            } else if mix != nil {
                Text("No saved songs.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                DotRow(tones: Array(repeating: Tone.grille.color(for: scheme), count: 30))
            }
        }
        .panel()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "Reached rotation"))
        .accessibilityValue(mix.map(spokenSummary) ?? "")
        .chartSummary(mix.flatMap(chartSummary))
    }

    private func tones(_ mix: RotationMix) -> [Color] {
        zip(RotationMix.Part.allCases, mix.runs()).flatMap { part, run in
            Array(repeating: Self.tone(part, scheme: scheme), count: run)
        }
    }

    @ViewBuilder private func legend(_ mix: RotationMix) -> some View {
        let shown = mix.shown
        FlowLayout(spacing: 14, lineSpacing: 6) {
            if shown.count == 1, let only = shown.first {
                LegendItem(color: Self.tone(only, scheme: scheme), name: Self.all(only), share: nil)
            } else {
                ForEach(shown, id: \.self) { part in
                    LegendItem(
                        color: Self.tone(part, scheme: scheme), name: Self.name(part),
                        share: RotationMix.percent(mix.share(part)))
                }
            }
        }
    }

    // MARK: - Parts

    /// Before saving apart, the lit steps from a day to a month, then
    /// aluminium for later and the unlit grille for not yet.
    static func tone(_ part: RotationMix.Part, scheme: ColorScheme) -> Color {
        let lamp = Tone.lamp.color(for: scheme)
        return switch part {
        case .beforeSaving: Tone.series[1].color(for: scheme)
        case .withinDay: lamp
        case .withinWeek: lamp.opacity(0.7)
        case .withinMonth: lamp.opacity(0.45)
        case .later: Tone.series[3].color(for: scheme)
        case .notYet: Tone.grille.color(for: scheme)
        }
    }

    static func name(_ part: RotationMix.Part) -> String {
        switch part {
        case .beforeSaving: String(localized: "Before saving")
        case .withinDay: String(localized: "Within a day")
        case .withinWeek: String(localized: "Within a week")
        case .withinMonth: String(localized: "Within a month")
        case .later: String(localized: "Later")
        case .notYet: String(localized: "Not yet")
        }
    }

    /// Said instead of the legend when one part is everything.
    static func all(_ part: RotationMix.Part) -> String {
        switch part {
        case .beforeSaving: String(localized: "All before saving")
        case .withinDay: String(localized: "All within a day")
        case .withinWeek: String(localized: "All within a week")
        case .withinMonth: String(localized: "All within a month")
        case .later: String(localized: "All later")
        case .notYet: String(localized: "None yet")
        }
    }

    // MARK: - Words

    /// "Before saving 3%, Within a day under 1%, …"
    private func spokenSummary(_ mix: RotationMix) -> String {
        guard mix.total > 0 else { return String(localized: "No saved songs.") }
        return mix.shown
            .map { "\(Self.name($0)) \(RotationMix.percent(mix.share($0)))" }
            .joined(separator: ", ")
    }

    private func chartSummary(_ mix: RotationMix) -> ChartSummary? {
        guard mix.total > 0 else { return nil }
        let title = String(localized: "Reached rotation")
        return ChartSummary(
            title: title, summary: spokenSummary(mix), xTitle: String(localized: "When"),
            yTitle: String(localized: "Songs"), categories: RotationMix.Part.allCases.map(Self.name),
            series: [ChartSummary.Series(name: title, values: mix.counts.map(Double.init))],
            range: 0...Double(mix.counts.max() ?? 1), format: .count)
    }
}

/// A row of dots across the panel, one colour each.
private struct DotRow: View {
    let tones: [Color]

    var body: some View {
        Canvas { context, size in
            guard !tones.isEmpty else { return }
            let pitch = size.width / CGFloat(tones.count)
            let side = min(6, pitch * 0.7)
            for (index, tone) in tones.enumerated() {
                let rect = CGRect(
                    x: pitch * CGFloat(index) + (pitch - side) / 2, y: (size.height - side) / 2, width: side,
                    height: side)
                context.fill(Path(ellipseIn: rect), with: .color(tone))
            }
        }
        .frame(height: 8)
        .accessibilityHidden(true)
    }
}

/// A dot, a part's name and its share.
private struct LegendItem: View {
    let color: Color
    let name: String
    let share: String?

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(color)
                .frame(width: 6, height: 6)
            Text(name)
                .foregroundStyle(.secondary)
            if let share {
                Text(share)
                    .monospacedDigit()
            }
        }
        .font(.caption)
    }
}
