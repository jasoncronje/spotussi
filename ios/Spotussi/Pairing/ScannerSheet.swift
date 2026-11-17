import SpotussiKit
import SwiftUI

/// The camera, looking for the pairing code, or why it can't.
struct ScannerSheet: View {
    let onLink: (PairingLink) -> Void
    let onEnterByHand: () -> Void

    @Environment(PairingModel.self) private var pairing
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    @State private var camera = CameraAccess()
    /// The last code read wasn't a pairing code.
    @State private var readWrongCode = false

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Scan pairing code")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(role: .close) { dismiss() }
                    }
                }
        }
        .task { await camera.check() }
    }

    @ViewBuilder private var content: some View {
        switch camera.state {
        case .checking:
            Color.clear
        case .ready:
            QRScanner(onCode: read, onUnavailable: camera.cameraWentAway)
                .ignoresSafeArea()
                .overlay(alignment: .bottom) { guidance }
        case .denied:
            ContentUnavailableView {
                Label("Camera access is off", systemImage: "camera")
            } description: {
                Text("Allow the camera in Settings, or type the address and token.")
            } actions: {
                Button {
                    if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
                } label: {
                    Text("Open Settings").foregroundStyle(.accentLabel)
                }
                .buttonStyle(.borderedProminent)
                Button("Enter address and token", action: onEnterByHand)
                    .tint(.primary)
            }
        case .unavailable:
            ContentUnavailableView {
                Label("No camera to scan with", systemImage: "qrcode.viewfinder")
            } description: {
                Text("This device can’t scan. Type the address and token.")
            } actions: {
                Button(action: onEnterByHand) {
                    Text("Enter address and token").foregroundStyle(.accentLabel)
                }
                .buttonStyle(.borderedProminent)
            }
        }
    }

    private var guidance: some View {
        VStack(alignment: .leading, spacing: 4) {
            if readWrongCode {
                Label("That isn’t a Spotussi pairing code.", systemImage: "exclamationmark.triangle.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.orange)
            }
            Text("Point at the code in Settings, API & MCP on the web.")
                .font(.subheadline)
        }
        .panel()
        .padding(16)
        .accessibilityElement(children: .combine)
    }

    private func read(_ payload: String) {
        if let link = pairing.link(fromScanned: payload) {
            onLink(link)
        } else {
            readWrongCode = true
            AccessibilityNotification.Announcement(
                String(localized: "That isn’t a Spotussi pairing code.")
            ).post()
        }
    }
}
