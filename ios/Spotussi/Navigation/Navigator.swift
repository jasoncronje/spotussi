import Observation
import SpotussiKit
import SwiftUI

/// The selected tab and each tab's stack of pages, held above the tabs so
/// a `spotussi://artist/<id>` link (album, track, genre, playlist) can push
/// its page onto the tab in front.
@Observable
final class Navigator {
    nonisolated enum Tab: Hashable {
        case overview, top, stats, history, library
    }

    var selection = Tab.overview
    private var paths: [Tab: [Route]] = [:]

    func path(for tab: Tab) -> Binding<[Route]> {
        Binding(get: { self.paths[tab] ?? [] }, set: { self.paths[tab] = $0 })
    }

    /// Pushes the link's page onto the tab in front.
    func open(_ link: DeepLink) {
        let route: Route =
            switch link {
            case .entity(let entity): .entity(entity)
            case .playlist(let id, let name): .playlist(PlaylistLink(id: id, name: name))
            }
        paths[selection, default: []].append(route)
    }

    /// Back to the first tab with nothing pushed, for a new connection.
    func reset() {
        selection = .overview
        paths = [:]
    }
}

private nonisolated struct TabKey: EnvironmentKey {
    static let defaultValue: Navigator.Tab? = nil
}

extension EnvironmentValues {
    /// The tab a view is in; nil outside the tab bar (previews, sheets).
    nonisolated var tab: Navigator.Tab? {
        get { self[TabKey.self] }
        set { self[TabKey.self] = newValue }
    }
}
