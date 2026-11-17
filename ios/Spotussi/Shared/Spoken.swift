import SpotussiKit
import SwiftUI

/// Figures as VoiceOver should read them: "36 plays", "2 hours, 5 minutes".
enum Spoken {
    static func plays(_ count: Int) -> String {
        String(localized: "\(count) plays")
    }

    static func value(_ value: Int, measure: Measure) -> String {
        switch measure {
        case .plays: plays(value)
        case .time: Figures.duration(milliseconds: value, width: .wide)
        }
    }
}

/// A section whose request failed: one quiet line, fixed by pulling down.
struct SectionFailure: View {
    var body: some View {
        Text("Couldn’t load. Pull down to retry.")
            .font(.subheadline)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// A panel's title.
struct PanelTitle: View {
    let title: LocalizedStringKey

    init(_ title: LocalizedStringKey) {
        self.title = title
    }

    var body: some View {
        Text(title)
            .font(.headline)
            .accessibilityAddTraits(.isHeader)
    }
}
