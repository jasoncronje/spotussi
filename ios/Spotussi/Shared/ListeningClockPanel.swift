import SpotussiKit
import SwiftUI

/// The listening clock on a panel: the dial, a readout of the busiest hour
/// or the one picked, and on Stats that hour's top artist and track, which
/// open their pages. VoiceOver reads the hour and swipes through the day.
struct ListeningClockPanel: View {
    let hours: HourlyListening?
    let isFailed: Bool
    /// The measure to title the readout with before the hours are in.
    let measure: Measure
    @State private var selected: Int?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline) {
                    PanelTitle("Listening clock")
                    Spacer(minLength: 12)
                    quietest
                }
                VStack(alignment: .leading, spacing: 2) {
                    PanelTitle("Listening clock")
                    quietest
                }
            }
            if isFailed {
                SectionFailure()
            } else if let hours, hours.isEmpty {
                Text("No plays in this range.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                dial
                leaders
            }
        }
        .panel()
    }

    // MARK: - Dial

    @ViewBuilder private var dial: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: 12) {
                ListeningClockDial(hours: hours, selected: $selected)
                    .frame(maxWidth: .infinity)
                readout(alignment: .leading)
            }
            .modifier(DialAccessibility(hours: hours, shown: shown, selection: $selected))
        } else {
            ListeningClockDial(hours: hours, selected: $selected)
                // Inside the innermost ring.
                .overlay { readout(alignment: .center).frame(maxWidth: 104) }
                .frame(maxWidth: .infinity)
                .modifier(DialAccessibility(hours: hours, shown: shown, selection: $selected))
        }
    }

    /// The hour shown: the one picked, or the busiest.
    private var shown: HourlyListening.Hour? {
        guard let hours else { return nil }
        if let selected { return hours.hours[selected] }
        return hours.peak
    }

    private func readout(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 2) {
            Text(selected == nil ? String(localized: "Busiest hour") : shown.map { Figures.hourSpan($0.id) } ?? "")
                .font(.caption2)
                .foregroundStyle(.secondary)
            Text(figure)
                .font(.readout(.title2))
                .contentTransition(.numericText())
            Text(shareText)
                .font(.caption2.monospacedDigit())
                .foregroundStyle(.secondary)
        }
        .multilineTextAlignment(alignment == .center ? .center : .leading)
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .redacted(reason: hours == nil ? .placeholder : [])
        .allowsHitTesting(false)
    }

    /// The busiest hour by its clock time; a picked hour by its figure.
    private var figure: String {
        guard let hours, let hour = shown else { return Figures.hour(10) }
        guard selected != nil else { return Figures.hour(hour.id) }
        return value(hours.value(hour))
    }

    /// "6% of plays"
    private var shareText: String {
        let percent = Figures.percent(shown.map { hours?.share($0) ?? 0 } ?? 0)
        return measureShown == .plays
            ? String(localized: "\(percent) of plays") : String(localized: "\(percent) of time")
    }

    @ViewBuilder private var quietest: some View {
        if let quietest = hours?.quietest {
            Text("Quietest \(Figures.hour(quietest.id))")
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
        }
    }

    // MARK: - Leaders

    @ViewBuilder private var leaders: some View {
        if let hour = shown, hour.topArtist != nil || hour.topTrack != nil {
            VStack(spacing: 2) {
                if let artist = hour.topArtist {
                    LeaderLink(role: .artist, leader: artist, figure: value(artist.value(for: measureShown)))
                }
                if let track = hour.topTrack {
                    LeaderLink(role: .track, leader: track, figure: value(track.value(for: measureShown)))
                }
            }
        }
    }

    private var measureShown: Measure { hours?.measure ?? measure }

    private func value(_ value: Int) -> String {
        measureShown == .plays ? Figures.count(value) : Figures.duration(milliseconds: value)
    }
}

/// The dial as one adjustable element: its value is the hour shown, and
/// swiping up or down moves through the day.
private struct DialAccessibility: ViewModifier {
    let hours: HourlyListening?
    let shown: HourlyListening.Hour?
    let selection: Binding<Int?>

    func body(content: Content) -> some View {
        content
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("Listening clock"))
            .accessibilityValue(value)
            .accessibilityAdjustableAction { direction in
                let current = selection.wrappedValue ?? shown?.id ?? 0
                switch direction {
                case .increment: selection.wrappedValue = (current + 1) % 24
                case .decrement: selection.wrappedValue = (current + 23) % 24
                @unknown default: break
                }
            }
            .chartSummary(summary)
    }

    /// "2 – 3 PM, 91 plays, 7% of plays"
    private var value: String {
        guard let hours, let hour = shown else { return "" }
        let percent = Figures.percent(hours.share(hour))
        let share =
            hours.measure == .plays
            ? String(localized: "\(percent) of plays") : String(localized: "\(percent) of time")
        return [Figures.hourSpan(hour.id), Spoken.value(hours.value(hour), measure: hours.measure), share].joined(separator: ", ")
    }

    private var summary: ChartSummary? {
        guard let hours, let peak = hours.peak, let quietest = hours.quietest else { return nil }
        let title = String(localized: "Listening clock")
        return ChartSummary(
            title: title,
            summary: String(
                localized: "Busiest around \(Figures.hour(peak.id)), quietest around \(Figures.hour(quietest.id))."),
            xTitle: String(localized: "Hour"), yTitle: hours.measure.label,
            categories: hours.hours.map { Figures.hourSpan($0.id) },
            series: [ChartSummary.Series(name: title, values: hours.hours.map { Double(hours.value($0)) })],
            range: 0...Double(max(1, hours.peak.map(hours.value) ?? 1)),
            format: hours.measure == .plays ? .count : .duration)
    }
}

/// An hour's top artist or track, opening its page.
private struct LeaderLink: View {
    enum Role {
        case artist, track
    }

    let role: Role
    let leader: HourlyListening.Leader
    let figure: String
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        RouteLink(
            route: .entity(leader.link),
            accessibilityLabel: [roleText, leader.link.name, leader.detail].compactMap(\.self).joined(separator: ", "),
            accessibilityValue: figure
        ) {
            let layout =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
                : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
            layout {
                ArtworkImage(
                    url: leader.imageURL, side: 40, symbol: leader.link.kind.symbol, isRound: role == .artist)
                VStack(alignment: .leading, spacing: 2) {
                    Text(roleText)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    Text(leader.link.name)
                        .font(.body.weight(.medium))
                    if let detail = leader.detail {
                        Text(detail)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                Text(figure)
                    .font(.subheadline.weight(.semibold).monospacedDigit())
                    .fixedSize()
            }
            .padding(.vertical, 6)
            .frame(minHeight: 44)
            .contentShape(.rect)
        }
    }

    private var roleText: String {
        role == .artist ? String(localized: "Top artist") : String(localized: "Top track")
    }
}
