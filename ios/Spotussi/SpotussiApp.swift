import SpotussiKit
import SwiftUI

@main
struct SpotussiApp: App {
    @State private var session: Session
    @State private var pairing: PairingModel
    @State private var options = ViewOptions()

    init() {
        let session = Session()
        _session = State(initialValue: session)
        _pairing = State(initialValue: PairingModel(session: session))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(session)
                .environment(pairing)
                .environment(options)
        }
    }
}
