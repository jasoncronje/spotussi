import Foundation

/// A link into a page of the app:
///
///     spotussi://artist/<id>
///     spotussi://album/<id>
///     spotussi://track/<id>
///     spotussi://genre/<id>
///     spotussi://playlist/<id>
///
/// An optional `name` query item titles the page until it loads.
/// Pairing links (`spotussi://pair`) are `PairingLink`'s.
public enum DeepLink: Hashable, Sendable {
    case entity(EntityLink)
    case playlist(id: String, name: String)

    public init?(_ url: URL) {
        guard url.scheme?.lowercased() == "spotussi", let host = url.host()?.lowercased() else { return nil }
        let parts = url.pathComponents.filter { $0 != "/" }
        guard parts.count == 1, let id = parts.first, !id.isEmpty else { return nil }
        let name =
            URLComponents(url: url, resolvingAgainstBaseURL: false)?
            .queryItems?.first { $0.name == "name" }?.value ?? ""
        if host == "playlist" {
            self = .playlist(id: id, name: name)
        } else if let kind = EntityKind(rawValue: host) {
            self = .entity(EntityLink(kind: kind, id: id, name: name))
        } else {
            return nil
        }
    }
}
