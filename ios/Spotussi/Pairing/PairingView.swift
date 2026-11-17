import SpotussiKit
import SwiftUI

/// The first screen: what the app is, and the three ways in.
struct PairingView: View {
    @Environment(Session.self) private var session
    @Environment(PairingModel.self) private var pairing
    @State private var isScanning = false
    @State private var isEnteringByHand = false
    /// Set by the scanner when this device can't scan, so the typed entry
    /// opens once the scanner sheet has gone.
    @State private var opensEntryAfterScanner = false

    var body: some View {
        ScrollView {
            VStack(spacing: 36) {
                BrandHeader()
                intro
                if let problem = pairing.problem {
                    ProblemMessage(problem: problem)
                        .panel()
                        .transition(.opacity)
                }
                actions
                Text("Read-only access. The token is kept in the Keychain on this iPhone.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, 24)
            .padding(.vertical, 32)
            .frame(maxWidth: 520)
            .frame(maxWidth: .infinity)
        }
        .defaultScrollAnchor(.center, for: .alignment)
        .scrollBounceBehavior(.basedOnSize)
        .background(.ground)
        .sheet(isPresented: $isScanning, onDismiss: openEntryIfAsked) {
            ScannerSheet(
                onLink: { link in
                    isScanning = false
                    pairing.pair(link)
                },
                onEnterByHand: {
                    opensEntryAfterScanner = true
                    isScanning = false
                })
        }
        .sheet(isPresented: $isEnteringByHand) {
            ManualEntrySheet()
        }
    }

    private var intro: some View {
        VStack(spacing: 12) {
            Text("Connect to your server")
                .font(.system(.title2, weight: .bold).width(.expanded))
                .accessibilityAddTraits(.isHeader)
            Text("Your listening stats from your own Spotussi server, on your iPhone.")
                .foregroundStyle(.secondary)
            Text("On the web: **Settings**, **API & MCP**, **Pair iPhone**.")
                .foregroundStyle(.secondary)
        }
        .multilineTextAlignment(.center)
    }

    private var actions: some View {
        VStack(spacing: 14) {
            ScanButton(connectingTo: pairing.connectingTo) {
                pairing.dismissProblem()
                isScanning = true
            }
            Button {
                pairing.dismissProblem()
                isEnteringByHand = true
            } label: {
                Text("Enter address and token")
                    .font(.body.weight(.semibold))
                    .frame(maxWidth: .infinity, minHeight: 52)
                    .padding(.horizontal, 20)
                    .contentShape(.capsule)
            }
            .buttonStyle(.plain)
            .glassEffect(.regular.interactive(), in: .capsule)
            .disabled(pairing.isConnecting)

            if pairing.isConnecting {
                Button("Cancel") { pairing.cancel() }
                    .buttonStyle(.borderless)
                    .tint(.secondary)
                    .frame(minHeight: 44)
            } else {
                Button("Try the demo") { session.startDemo() }
                    .buttonStyle(.borderless)
                    .tint(.secondary)
                    .font(.subheadline.weight(.medium))
                    .frame(minHeight: 44)
                    .accessibilityHint("Shows sample listening data instead of your own.")
            }
        }
    }

    private func openEntryIfAsked() {
        guard opensEntryAfterScanner else { return }
        opensEntryAfterScanner = false
        isEnteringByHand = true
    }
}

/// The primary action, which shows the connection in progress after a scan.
private struct ScanButton: View {
    let connectingTo: Instance?
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Group {
                // Disabled while connecting, so the system's dimmed colours
                // apply instead of the label colour meant for the green.
                if let instance = connectingTo {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Connecting to \(instance.displayName)…")
                    }
                } else {
                    Label("Scan QR code", systemImage: "qrcode.viewfinder")
                        .foregroundStyle(.accentLabel)
                }
            }
            .font(.body.weight(.semibold))
            .frame(maxWidth: .infinity, minHeight: 36)
        }
        .buttonStyle(.borderedProminent)
        .buttonBorderShape(.capsule)
        .controlSize(.large)
        .disabled(connectingTo != nil)
    }
}

/// The mark on its tile, and the name.
private struct BrandHeader: View {
    @ScaledMetric(relativeTo: .largeTitle) private var tileSize = 88

    var body: some View {
        VStack(spacing: 14) {
            LevelMark()
                .padding(tileSize * 0.2)
                .frame(width: tileSize, height: tileSize)
                .background(.panel, in: .rect(cornerRadius: tileSize * 0.26))
            Text(verbatim: "spotussi")
                .font(.system(.title3, weight: .bold).width(.expanded))
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(verbatim: "Spotussi"))
        .accessibilityAddTraits(.isHeader)
    }
}

#Preview("Idle") {
    PreviewHost(.notPaired) { PairingView() }
}

#Preview("Error") {
    PreviewHost(.notPaired, pairing: .failed(.pairing(.tokenRejected))) { PairingView() }
}

#Preview("Connecting") {
    PreviewHost(.notPaired, pairing: .connecting(Instance(address: "http://192.168.1.20:8080")!)) {
        PairingView()
    }
}
