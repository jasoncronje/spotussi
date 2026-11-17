import SpotussiKit
import SwiftUI

/// The log's filters in one toolbar menu, as the web's History offers
/// them: an artist, album or track, skipped, shuffle, counted, and a device
/// or platform. The control shows how many are on.
struct HistoryFilterMenu: View {
    @Binding var filter: HistoryFilter
    let devices: PlaybackDevices?
    /// Opens the artist, album or track search; nil where the page is
    /// already about one entity, which is then not a filter to change.
    var pickScope: (() -> Void)?

    var body: some View {
        Menu {
            if let pickScope {
                Section {
                    Button(action: pickScope) {
                        Label("Artist, album or track", systemImage: "magnifyingglass")
                        if let scope = filter.scope {
                            Text(scope.name)
                        }
                    }
                }
            }
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
            if count > 0 {
                Section {
                    Button("Clear filters", systemImage: "xmark") {
                        filter = filter.withoutFilters(includingScope: pickScope != nil)
                    }
                }
            }
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "line.3.horizontal.decrease")
                if count > 0 {
                    Text(count, format: .number)
                        .font(.subheadline.weight(.semibold).monospacedDigit())
                }
            }
        }
        .accessibilityLabel("Filters")
        .accessibilityValue(count > 0 ? Text("\(count) on") : Text(verbatim: ""))
    }

    private var count: Int {
        filter.filterCount(includingScope: pickScope != nil)
    }
}

/// The filters that are on, each a chip that takes it off, and one to take
/// them all off. Nothing when none is on.
struct ActiveFilters: View {
    @Binding var filter: HistoryFilter
    /// The artist, album or track is a filter here, not the page's subject.
    var scopeIsFilter = false

    var body: some View {
        if filter.filterCount(includingScope: scopeIsFilter) > 0 {
            FlowLayout {
                if scopeIsFilter, let scope = filter.scope {
                    FilterChip(label: scope.name, symbol: scope.kind.symbol) {
                        filter.scope = nil
                    }
                }
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
                    filter = filter.withoutFilters(includingScope: scopeIsFilter)
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
    var symbol: String?
    let remove: () -> Void

    var body: some View {
        Button(action: remove) {
            HStack(spacing: 6) {
                if let symbol {
                    Image(systemName: symbol)
                        .foregroundStyle(.secondary)
                }
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

extension HistoryFilter {
    /// The filters that are on, counting the artist, album or track when a
    /// person chose it rather than the page.
    func filterCount(includingScope scope: Bool) -> Int {
        activeCount + (scope && self.scope != nil ? 1 : 0)
    }

    /// Every filter off; the artist, album or track too when a person
    /// chose it.
    func withoutFilters(includingScope scope: Bool) -> HistoryFilter {
        scope ? HistoryFilter() : cleared
    }
}
