import Foundation
import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Hosts a preview with everything the app puts in the environment. Nothing
/// touches the Keychain, the standard defaults or the network: the token
/// lives in memory and every request is answered by the demo data.
struct PreviewHost<Content: View>: View {
    enum Mode {
        case notPaired
        case demo
        /// Paired with a made-up address that the demo data answers.
        case paired
    }

    private let mode: Mode
    private let content: Content
    @State private var session: Session
    @State private var pairing: PairingModel
    @State private var options: ViewOptions
    @State private var status = InstanceStatus(client: .demo)
    @State private var nowPlaying = NowPlayingModel(client: .demo)
    @State private var navigator = Navigator()

    init(
        _ mode: Mode, pairing phase: PairingModel.Phase = .idle,
        @ViewBuilder content: () -> Content
    ) {
        let session = Session(
            tokens: InMemoryTokenStore(), defaults: Self.defaults, urlCache: nil, artworkCache: nil,
            makeClient: { _, _ in .demo })
        if mode == .demo { session.startDemo() }
        self.mode = mode
        self.content = content()
        _session = State(initialValue: session)
        _pairing = State(initialValue: PairingModel(session: session, phase: phase))
        _options = State(initialValue: ViewOptions(defaults: Self.defaults))
    }

    var body: some View {
        content
            .environment(session)
            .environment(pairing)
            .environment(options)
            .environment(status)
            .environment(nowPlaying)
            .environment(navigator)
            .task {
                guard mode == .paired,
                    let link = try? PairingLink(address: "https://stats.example.com", token: "spt_preview")
                else { return }
                try? await session.pair(link)
            }
    }

    private static var defaults: UserDefaults {
        UserDefaults(suiteName: "app.spotussi.ios.previews") ?? .standard
    }
}
