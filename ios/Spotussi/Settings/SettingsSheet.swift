import SpotussiKit
import SwiftUI

/// The instance, its state as the server reports it, leaving, and about.
/// Read-only: every setting lives on the web.
struct SettingsSheet: View {
    @Environment(Session.self) private var session
    @Environment(InstanceStatus.self) private var status
    @Environment(\.dismiss) private var dismiss
    @State private var isConfirmingUnpair = false

    var body: some View {
        NavigationStack {
            Form {
                serverSection
                if !session.isDemo {
                    StatusSection(status: status)
                }
                leaveSection
                aboutSection
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(role: .close) { dismiss() }
                }
            }
            .task { await status.refresh() }
            .refreshable { [status] in await status.refresh() }
        }
    }

    // MARK: - Sections

    @ViewBuilder private var serverSection: some View {
        Section("Server") {
            if let instance = session.instance {
                VStack(alignment: .leading, spacing: 4) {
                    Text(instance.displayName)
                        .font(.headline)
                    Text(instance.baseURL.absoluteString)
                        .font(.subheadline.monospaced())
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
                .padding(.vertical, 4)
                .accessibilityElement(children: .combine)
            } else {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Demo data")
                        .font(.headline)
                    Text("Sample listening, not real history.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .padding(.vertical, 4)
                .accessibilityElement(children: .combine)
            }
        }
    }

    @ViewBuilder private var leaveSection: some View {
        if session.isDemo {
            Section {
                Button("Leave demo") {
                    dismiss()
                    session.unpair()
                }
            } footer: {
                Text("Returns to pairing.")
            }
        } else {
            Section {
                Button("Unpair this iPhone", role: .destructive) {
                    isConfirmingUnpair = true
                }
                .confirmationDialog(
                    "Unpair this iPhone?", isPresented: $isConfirmingUnpair,
                    titleVisibility: .visible
                ) {
                    Button("Unpair", role: .destructive) {
                        dismiss()
                        session.unpair()
                    }
                    Button("Cancel", role: .cancel) {}
                } message: {
                    Text(
                        "The token stays valid until you revoke it on the web."
                    )
                }
            }
        }
    }

    private var aboutSection: some View {
        Section("About") {
            LabeledContent("Version", value: AppVersion.display)
            Text("Free software, AGPL-3.0.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Text("Not affiliated with Spotify.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
    }
}

#Preview("Demo") {
    PreviewHost(.demo) {
        Color.clear.sheet(isPresented: .constant(true)) { SettingsSheet() }
    }
}

#Preview("Paired") {
    PreviewHost(.paired) {
        Color.clear.sheet(isPresented: .constant(true)) { SettingsSheet() }
    }
}
