import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The instance's notices, most pressing first, at the top of every tab.
/// A pairing link opened while connected shows its progress here too.
struct StatusNotices: View {
    @Environment(InstanceStatus.self) private var status
    @Environment(PairingModel.self) private var pairing

    var body: some View {
        if let instance = pairing.connectingTo {
            ConnectingPanel(instance: instance)
        }
        ForEach(status.notices) { notice in
            NoticePanel(notice: notice)
        }
    }
}

private struct ConnectingPanel: View {
    let instance: Instance

    var body: some View {
        HStack(spacing: 12) {
            ProgressView()
            Text("Connecting to \(instance.displayName)…")
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .panel()
        .accessibilityElement(children: .combine)
    }
}

#if DEBUG
/// The demo instance is quiet, so this builds the notices from its status
/// with Spotify disconnected and lookups part-way.
#Preview("Notices") {
    let notices: [StatusNotice] = {
        guard let data = Fixtures.data(for: "getSystemStatus"),
            var status = try? JSONDecoder().decode(Components.Schemas.SystemStatus.self, from: data)
        else { return [] }
        status.spotify.state = .reconnectRequired
        status.catalog.state = .hydrating
        status.catalog.tracksTotal = 1000
        status.catalog.tracksHydrated = 405
        var quiet = status
        quiet.spotify.state = .notConfigured
        return StatusNotice.notices(for: status, now: .now)
            + StatusNotice.notices(for: quiet, now: .now).filter { $0.kind == .spotifyNotConnected }
    }()
    ScrollView {
        VStack(spacing: 16) {
            ForEach(notices) { NoticePanel(notice: $0) }
        }
        .padding(16)
    }
    .background(.ground)
}
#endif
