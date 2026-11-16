import Foundation

/// The link the web app shows as a QR code under Settings → API & MCP:
///
///     spotussi://pair?url=<base URL>&token=<spt_…>
///
/// `url` is the instance's origin. The token is a read-only API token.
public struct PairingLink: Equatable, Sendable {
    public let instance: Instance
    public let token: String

    public enum Failure: Error, Equatable, Sendable {
        /// Not a `spotussi://pair` link at all.
        case notAPairingLink
        /// The address is missing or isn't an http(s) URL with a host.
        case badAddress
        /// The token is missing or doesn't look like a Spotussi API token.
        case badToken
    }

    public init(_ url: URL) throws(Failure) {
        guard url.scheme?.lowercased() == "spotussi", url.host()?.lowercased() == "pair",
            let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems
        else { throw .notAPairingLink }
        func value(_ name: String) -> String? {
            items.first { $0.name == name }?.value
        }
        guard let address = value("url"), let instance = Instance(address: address) else {
            throw .badAddress
        }
        guard let token = value("token"), Self.looksLikeToken(token) else { throw .badToken }
        self.instance = instance
        self.token = token
    }

    /// Typed in by hand: an address and a token.
    public init(address: String, token: String) throws(Failure) {
        guard let instance = Instance(address: address) else { throw .badAddress }
        let token = token.trimmingCharacters(in: .whitespacesAndNewlines)
        guard Self.looksLikeToken(token) else { throw .badToken }
        self.instance = instance
        self.token = token
    }

    static func looksLikeToken(_ token: String) -> Bool {
        token.hasPrefix("spt_") && token.count > 8
            && token.unicodeScalars.allSatisfy { $0.isASCII && !$0.properties.isWhitespace }
    }
}

/// One Spotussi server, named by the address a browser would open.
public struct Instance: Hashable, Codable, Sendable {
    /// The origin, with no trailing slash, path, query or fragment.
    public let baseURL: URL

    /// Accepts what people type: `stats.example.com`, `http://10.0.0.5:8080/`,
    /// a full page address. A missing scheme means https.
    public init?(address: String) {
        var text = address.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return nil }
        if !text.contains("://") { text = "https://" + text }
        guard var parts = URLComponents(string: text),
            let scheme = parts.scheme?.lowercased(), scheme == "http" || scheme == "https",
            let host = parts.host, !host.isEmpty
        else { return nil }
        parts.scheme = scheme
        parts.user = nil
        parts.password = nil
        parts.path = ""
        parts.query = nil
        parts.fragment = nil
        guard let url = parts.url else { return nil }
        baseURL = url
    }

    /// Where the API lives: `<origin>/api/v1`.
    public var apiURL: URL { baseURL.appending(path: "api/v1") }

    /// The host (and port, when it isn't the default) for display.
    public var displayName: String {
        guard let host = baseURL.host() else { return baseURL.absoluteString }
        return baseURL.port.map { "\(host):\($0)" } ?? host
    }

    /// Plain http: fine on a home network, worth a note anywhere else.
    public var isPlainHTTP: Bool { baseURL.scheme == "http" }
}
