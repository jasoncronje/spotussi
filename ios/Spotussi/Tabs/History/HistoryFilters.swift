import SpotussiKit
import SwiftUI

/// The log's filters in one toolbar menu, as the web's History offers
/// them: skipped, shuffle, counted, and a device or platform. The control
/// shows how many are on.
struct HistoryFilterMenu: View {
    @Binding var filter: HistoryFilter
    let devices: PlaybackDevices?

    var body: some View {
        Menu {
            Picker("Skipped", selection: $filter.skipped) {
                Text("Any").tag(Bool?.none)
                Text("Skipped").tag(Bool?.some(true))
                Text("Not skipped").tag(Bool?.some(false))
            }
            .pickerStyle(.menu)
            Picker("Shuffle", selection: $filter.shuffle) {
                Text("Any").tag(Bool?.none)
                Text("Shuffle on").tag(Bool?.some(true))
                Text("Shuffle off").tag(Bool?.some(false))
            }
            .pickerStyle(.menu)
            Picker("Counted", selection: $filter.counted) {
                Text("Any").tag(Bool?.none)
                Text("Counted").tag(Bool?.some(true))
                Text("Not counted").tag(Bool?.some(false))
            }
            .pickerStyle(.menu)
            if let devices, !devices.isEmpty {
                Picker("Device", selection: $filter.device) {
                    Text("Any").tag(HistoryFilter.Device?.none)
                    if !devices.devices.isEmpty {
                        Section("Devices") {
                            ForEach(devices.devices, id: \.self) { name in
                                Text(name).tag(HistoryFilter.Device?.some(.device(name)))
                            }
                        }
                    }
                    if !devices.platforms.isEmpty {
                        Section("Platforms") {
                            ForEach(devices.platforms, id: \.self) { name in
                                Text(name).tag(HistoryFilter.Device?.some(.platform(name)))
                            }
                        }
                    }
                }
                .pickerStyle(.menu)
            }
            if filter.activeCount > 0 {
                Section {
                    Button("Clear filters", systemImage: "xmark") {
                        filter = filter.cleared
                    }
                }
            }
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "line.3.horizontal.decrease")
                if filter.activeCount > 0 {
                    Text(filter.activeCount, format: .number)
                        .font(.subheadline.weight(.semibold).monospacedDigit())
                }
            }
        }
        .accessibilityLabel("Filters")
        .accessibilityValue(filter.activeCount > 0 ? Text("\(filter.activeCount) on") : Text(verbatim: ""))
    }
}

/// The filters that are on, each a chip that takes it off, and one to take
/// them all off. Nothing when none is on.
struct ActiveFilters: View {
    @Binding var filter: HistoryFilter

    var body: some View {
        if filter.activeCount > 0 {
            FlowLayout {
                if let skipped = filter.skipped {
                    FilterChip(label: skipped ? String(localized: "Skipped") : String(localized: "Not skipped")) {
                        filter.skipped = nil
                    }
                }
                if let shuffle = filter.shuffle {
                    FilterChip(label: shuffle ? String(localized: "Shuffle on") : String(localized: "Shuffle off")) {
                        filter.shuffle = nil
                    }
                }
                if let counted = filter.counted {
                    FilterChip(label: counted ? String(localized: "Counted") : String(localized: "Not counted")) {
                        filter.counted = nil
                    }
                }
                if let device = filter.device {
                    FilterChip(label: device.name) {
                        filter.device = nil
                    }
                }
                Button {
                    filter = filter.cleared
                } label: {
                    Text("Clear")
                        .font(.subheadline.weight(.medium))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 12)
                        .frame(minHeight: 44)
                        .contentShape(.capsule)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear filters")
            }
        }
    }
}

/// One filter that is on: its name and a cross, as a solid chip.
private struct FilterChip: View {
    let label: String
    let remove: () -> Void

    var body: some View {
        Button(action: remove) {
            HStack(spacing: 6) {
                Text(label)
                Image(systemName: "xmark")
                    .imageScale(.small)
                    .foregroundStyle(.secondary)
            }
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, 14)
            .frame(minHeight: 44)
            .background(.panel, in: .capsule)
            .contentShape(.capsule)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text("Remove \(label)"))
    }
}
