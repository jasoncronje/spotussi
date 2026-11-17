import SpotussiKit
import SwiftUI

/// The shared range: the web's quick picks first, the other presets in
/// their groups, then a custom span.
struct RangeMenu: View {
    @Environment(ViewOptions.self) private var options
    @State private var isPickingCustom = false

    var body: some View {
        @Bindable var options = options
        Menu {
            Picker("Range", selection: $options.range) {
                ForEach(StatsRange.Preset.quickPicks, id: \.self) { preset in
                    Text(preset.label).tag(StatsRange.preset(preset))
                }
            }
            .pickerStyle(.inline)
            .labelsHidden()

            Section {
                ForEach(Self.moreGroups) { group in
                    Picker(group.title, selection: $options.range) {
                        ForEach(group.presets, id: \.self) { preset in
                            Text(preset.label).tag(StatsRange.preset(preset))
                        }
                    }
                    .pickerStyle(.menu)
                }
            }

            Section {
                Button {
                    isPickingCustom = true
                } label: {
                    Label("Custom…", systemImage: options.range.isCustom ? "checkmark" : "calendar")
                    if options.range.isCustom {
                        Text(options.range.longLabel)
                    }
                }
            }
        } label: {
            HStack(spacing: 7) {
                Circle()
                    .fill(.lamp)
                    .frame(width: 7, height: 7)
                Text(options.range.shortLabel)
            }
        }
        .accessibilityLabel("Range")
        .accessibilityValue(options.range.longLabel)
        .sheet(isPresented: $isPickingCustom) {
            CustomRangeSheet()
        }
    }

    /// Groups with presets beyond the quick picks; the rest would repeat them.
    private static let moreGroups = StatsRange.Preset.groups.filter { group in
        !group.presets.allSatisfy(StatsRange.Preset.quickPicks.contains)
    }
}
