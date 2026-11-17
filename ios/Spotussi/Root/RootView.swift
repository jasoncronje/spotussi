import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Pairing until there is an instance (or the demo), then the tabs. Pairing
/// links are handled here so they work from any screen.
struct RootView: View {
    @Environment(Session.self) private var session
    @Environment(PairingModel.self) private var pairing
    /// A pairing link that would replace the paired instance, waiting for a yes.
    @State private var replacement: PairingLink?

    var body: some View {
        content
            .onOpenURL(perform: open)
            .sensoryFeedback(.success, trigger: pairing.pairings)
            .confirmationDialog(
                replacementTitle, isPresented: isConfirmingReplacement, titleVisibility: .visible,
                presenting: replacement
            ) { link in
                Button("Pair with \(link.instance.displayName)") {
                    pairing.pair(link)
                }
                Button("Cancel", role: .cancel) {}
            } message: { link in
                Text(replacementMessage(for: link))
            }
    }

    @ViewBuilder private var content: some View {
        if let client = session.client {
            ConnectedRoot(client: client)
                .id(connectionID)
                .alert(
                    pairing.problem?.title ?? "", isPresented: isShowingProblem,
                    presenting: pairing.problem
                ) { _ in
                    Button("OK") { pairing.dismissProblem() }
                } message: { problem in
                    Text(problem.advice)
                }
        } else {
            PairingView()
        }
    }

    /// A new instance, or a new token for the same one, starts the shell
    /// afresh, with its own status model.
    private var connectionID: String {
        "\(session.instance?.baseURL.absoluteString ?? "demo")#\(pairing.pairings)"
    }

    private func open(_ url: URL) {
        guard let link = pairing.link(from: url) else { return }
        if session.instance == nil {
            pairing.pair(link)
        } else {
            replacement = link
        }
    }

    // MARK: - Replacing the paired instance

    private var isConfirmingReplacement: Binding<Bool> {
        Binding(get: { replacement != nil }, set: { if !$0 { replacement = nil } })
    }

    private var replacementTitle: String {
        String(localized: "Replace the paired server?")
    }

    private func replacementMessage(for link: PairingLink) -> String {
        guard let current = session.instance, current != link.instance else {
            return String(localized: "This iPhone will use the new token for \(link.instance.displayName).")
        }
        return String(
            localized:
                "This iPhone will read from \(link.instance.displayName) instead of \(current.displayName). The old token stays valid until you revoke it on the web."
        )
    }

    private var isShowingProblem: Binding<Bool> {
        Binding(get: { pairing.problem != nil }, set: { if !$0 { pairing.dismissProblem() } })
    }
}

/// Owns the status and now-playing models for one connection, so they are
/// made once per instance rather than on every redraw.
private struct ConnectedRoot: View {
    @State private var status: InstanceStatus
    @State private var nowPlaying: NowPlayingModel

    init(client: Client) {
        _status = State(initialValue: InstanceStatus(client: client))
        _nowPlaying = State(initialValue: NowPlayingModel(client: client))
    }

    var body: some View {
        ShellView()
            .environment(status)
            .environment(nowPlaying)
    }
}
