import SpotussiKit
import SwiftUI

/// Two days, inclusive, up to today.
struct CustomRangeSheet: View {
    @Environment(ViewOptions.self) private var options
    @Environment(\.dismiss) private var dismiss
    @State private var from = Date.now
    @State private var to = Date.now
    @State private var isSeeded = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    DatePicker("From", selection: $from, in: ...to, displayedComponents: .date)
                    DatePicker("To", selection: $to, in: from...max(from, .now), displayedComponents: .date)
                } footer: {
                    Text("Both days count. Days follow your server’s time zone.")
                }
            }
            .navigationTitle("Custom range")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(role: .cancel) { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(role: .confirm) {
                        options.range = StatsRange(from: from, to: to)
                        dismiss()
                    }
                    .accessibilityLabel("Show this range")
                }
            }
        }
        .presentationDetents([.medium, .large])
        .onAppear(perform: seed)
    }

    /// Starts from the custom range in use, or the four weeks up to today.
    private func seed() {
        guard !isSeeded else { return }
        isSeeded = true
        if case .custom(let start, let end) = options.range,
            let start = start.date(), let end = end.date()
        {
            from = start
            to = end
        } else {
            to = .now
            from = Calendar.current.date(byAdding: .day, value: -27, to: .now) ?? .now
        }
    }
}
