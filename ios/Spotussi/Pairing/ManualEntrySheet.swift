import SpotussiKit
import SwiftUI

/// Pairing by hand: the address a browser would open and an API token.
struct ManualEntrySheet: View {
    @Environment(PairingModel.self) private var pairing
    @Environment(\.dismiss) private var dismiss
    @State private var address = ""
    @State private var token = ""
    @FocusState private var focus: Field?

    private enum Field { case address, token }

    var body: some View {
        NavigationStack {
            Form {
                addressSection
                tokenSection
                if let instance = pairing.connectingTo {
                    Section {
                        HStack(spacing: 12) {
                            ProgressView()
                            Text("Connecting to \(instance.displayName)…")
                                .foregroundStyle(.secondary)
                        }
                        .accessibilityElement(children: .combine)
                    }
                } else if let problem = pairing.problem {
                    Section {
                        ProblemMessage(problem: problem)
                    }
                }
            }
            .navigationTitle("Address and token")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(role: .cancel) {
                        pairing.cancel()
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Connect", action: connect)
                        .disabled(!canConnect)
                }
            }
            .onAppear { focus = .address }
            .onDisappear {
                if pairing.isConnecting { pairing.cancel() }
            }
        }
    }

    private var addressSection: some View {
        Section {
            TextField("stats.example.com", text: $address)
                .keyboardType(.URL)
                .textContentType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.next)
                .focused($focus, equals: .address)
                .onSubmit { focus = .token }
                .accessibilityLabel("Address")
        } header: {
            Text("Address")
        } footer: {
            if Instance(address: address)?.plainHTTPIsBlocked == true {
                Label(
                    "Plain http needs an IP address or .local name.",
                    systemImage: "exclamationmark.triangle"
                )
                .foregroundStyle(.orange)
            } else {
                Text("Same as in your browser.")
            }
        }
    }

    private var tokenSection: some View {
        Section {
            TextField("spt_…", text: $token, axis: .vertical)
                .font(.body.monospaced())
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.go)
                .focused($focus, equals: .token)
                .onSubmit(connect)
                .accessibilityLabel("Token")
            PasteButton(payloadType: String.self) { strings in
                if let pasted = strings.first {
                    token = pasted.trimmingCharacters(in: .whitespacesAndNewlines)
                }
            }
            .buttonBorderShape(.capsule)
        } header: {
            Text("Token")
        } footer: {
            Text("From Settings, API & MCP on the web.")
        }
    }

    private var canConnect: Bool {
        !pairing.isConnecting
            && !address.trimmingCharacters(in: .whitespaces).isEmpty
            && !token.trimmingCharacters(in: .whitespaces).isEmpty
    }

    private func connect() {
        guard canConnect else { return }
        focus = nil
        pairing.pair(address: address, token: token)
    }
}

#Preview {
    PreviewHost(.notPaired) {
        Color.clear.sheet(isPresented: .constant(true)) { ManualEntrySheet() }
    }
}
