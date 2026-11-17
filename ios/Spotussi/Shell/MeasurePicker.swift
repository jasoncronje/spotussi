import SpotussiKit
import SwiftUI

/// Plays or Time for one screen, remembered per screen (D7).
struct MeasurePicker: View {
    let screen: ViewOptions.Screen
    @Environment(ViewOptions.self) private var options

    var body: some View {
        Picker("Count by", selection: measure) {
            ForEach(Measure.allCases, id: \.self) { measure in
                Text(measure.label).tag(measure)
            }
        }
        .pickerStyle(.segmented)
        .fixedSize()
        .sensoryFeedback(.selection, trigger: options.measure(for: screen))
    }

    private var measure: Binding<Measure> {
        Binding(
            get: { options.measure(for: screen) },
            set: { options.setMeasure($0, for: screen) })
    }
}
