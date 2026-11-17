import SpotussiKit
import SwiftUI

/// A page a row opens. Every tab's stack and the search sheet's push the
/// same pages for the same route.
nonisolated enum Route: Hashable {
    /// An artist, album, track or genre page, titled with the link's name
    /// until it loads; zooming out of the row's artwork when `zoom` is set.
    case entity(EntityLink, zoom: ZoomOrigin? = nil)
    /// The whole ranking of one kind within an entity: an artist's tracks,
    /// a genre's albums.
    case ranked(EntityKind, within: EntityLink)
    /// The play log narrowed to an entity: "Plays in History".
    case history(EntityLink)
    /// One listening session and its plays in order.
    case session(SessionLink)
    /// A playlist in the library and its songs in order.
    case playlist(PlaylistLink)
}

/// A playlist to open, with its name to title the page until it loads.
nonisolated struct PlaylistLink: Hashable {
    let id: String
    let name: String
}

/// A session to open, with its start to title the page until it loads.
nonisolated struct SessionLink: Hashable {
    let id: String
    let startedAt: Date
}

/// The artwork a pushed page zooms out of: an id unique on its screen, in
/// that screen's namespace. Unique per row rather than per entity, so a
/// track listed twice zooms from the row that was tapped.
nonisolated struct ZoomOrigin: Hashable {
    let id: String
    let namespace: Namespace.ID
}

extension View {
    /// The pages routes open, for a navigation stack's root.
    func routeDestinations() -> some View {
        navigationDestination(for: Route.self) { route in
            RouteDestination(route: route)
        }
    }

    /// Marks this view as where `origin`'s page zooms from.
    func zoomSource(_ origin: ZoomOrigin?) -> some View {
        modifier(ZoomSource(origin: origin))
    }
}

private struct RouteDestination: View {
    let route: Route
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        switch route {
        case .entity(let link, let zoom):
            if let zoom, !reduceMotion {
                EntityPage(link: link)
                    .navigationTransition(.zoom(sourceID: zoom.id, in: zoom.namespace))
            } else {
                EntityPage(link: link)
            }
        case .ranked(let kind, let entity):
            ScopedTopPage(kind: kind, within: entity)
        case .history(let entity):
            ScopedHistoryPage(scope: entity)
        case .session(let link):
            SessionPage(link: link)
        case .playlist(let link):
            PlaylistPage(link: link)
        }
    }
}

private struct ZoomSource: ViewModifier {
    let origin: ZoomOrigin?

    func body(content: Content) -> some View {
        if let origin {
            content.matchedTransitionSource(id: origin.id, in: origin.namespace)
        } else {
            content
        }
    }
}

extension EntityKind {
    /// The screen whose Plays/Time choice this kind's page uses (D7).
    var screen: ViewOptions.Screen {
        switch self {
        case .artist: .artist
        case .album: .album
        case .track: .track
        case .genre: .genre
        }
    }

    /// The placeholder symbol for its artwork.
    var symbol: String {
        switch self {
        case .artist: "music.mic"
        case .album: "square.stack"
        case .track: "music.note"
        case .genre: "tag"
        }
    }
}
