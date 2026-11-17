import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Spotify, capture and the server, as `/system/status` and `/health` last
/// reported them.
struct StatusSection: View {
    let status: InstanceStatus

    var body: some View {
        Section {
            LabeledContent("Spotify", value: spotify)
            LabeledContent("Capture", value: capture)
            if let lastPlay {
                LabeledContent("Last play caught", value: lastPlay)
            }
            LabeledContent("Server version", value: status.serverVersion ?? unknown)
        } header: {
            Text("Status")
        } footer: {
            Text(updated)
        }
    }

    private var unknown: String {
        status.failure == nil
            ? String(localized: "Loading…")
            : String(localized: "Unknown")
    }

    private var spotify: String {
        guard let state = status.systemStatus?.spotify.state else { return unknown }
        switch state {
        case .connected:
            if let name = status.spotifyConnection?.accountName ?? status.systemStatus?.spotify.accountName {
                return String(localized: "Connected as \(name)")
            }
            return String(localized: "Connected")
        case .reconnectRequired:
            return String(localized: "Needs reconnecting on the web")
        case .notConfigured:
            return String(localized: "Not connected")
        }
    }

    private var capture: String {
        guard let state = status.systemStatus?.capture.state else { return unknown }
        return switch state {
        case .playing: String(localized: "Catching a play now")
        case .idle: String(localized: "Waiting for the next play")
        case .backingOff: String(localized: "Retrying after errors")
        case .stopped: String(localized: "Stopped")
        }
    }

    private var lastPlay: String? {
        guard let epoch = status.systemStatus?.capture.lastPlayAt else { return nil }
        let date = Date(timeIntervalSince1970: Double(epoch) / 1000)
        return date.formatted(.relative(presentation: .named))
    }

    private var updated: String {
        if status.isUnreachable {
            return String(localized: "Can’t reach the server right now.")
        }
        guard let lastUpdated = status.lastUpdated else { return "" }
        return String(
            localized: "Updated \(lastUpdated.formatted(.relative(presentation: .named))).")
    }
}
