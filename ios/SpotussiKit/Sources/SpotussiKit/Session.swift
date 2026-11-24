import Foundation
import Observation
import OpenAPIRuntime
import SpotussiAPI

/// Which instance the app reads from, if any: restored at launch, set by
/// pairing, cleared by unpairing.
@MainActor @Observable
public final class Session {
    public enum State {
        case notPaired
        case paired(Instance, Client)
        case demo
    }

    public private(set) var state: State = .notPaired

    // MARK: - Dependencies

    @ObservationIgnored private let tokens: any TokenStore
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let urlCache: URLCache?
    @ObservationIgnored private let artworkCache: URLCache?
    @ObservationIgnored private let makeClient: (Instance, String) -> Client
    /// The paired instance's token, for artwork the instance serves itself.
    @ObservationIgnored private var token: String?

    private static let addressKey = "instance.address"

    // MARK: - Initialization

    /// Restores the paired instance from the stored address and token.
    ///
    /// - Parameters:
    ///   - urlCache, artworkCache: emptied on unpairing, so nothing of the
    ///     old instance stays on the phone.
    ///   - makeClient: builds the client for an instance and token; tests
    ///     pass one over a stub transport.
    public init(
        tokens: any TokenStore = KeychainTokenStore(),
        defaults: UserDefaults = .standard,
        urlCache: URLCache? = .spotussiAPI,
        artworkCache: URLCache? = URLSession.artwork.configuration.urlCache,
        makeClient: @escaping (Instance, String) -> Client = { Client.live($0, token: $1) }
    ) {
        self.tokens = tokens
        self.defaults = defaults
        self.urlCache = urlCache
        self.artworkCache = artworkCache
        self.makeClient = makeClient
        restore()
    }

    // MARK: - Public

    /// The client for the current state: the paired instance, or the bundled
    /// sample data in demo mode.
    public var client: Client? {
        switch state {
        case .notPaired: nil
        case .paired(_, let client): client
        case .demo: .demo
        }
    }

    public var instance: Instance? {
        if case .paired(let instance, _) = state { instance } else { nil }
    }

    /// What to call the instance on screen: its host, or "Demo".
    public var displayName: String? {
        switch state {
        case .notPaired: nil
        case .paired(let instance, _): instance.displayName
        case .demo: String(localized: "Demo", bundle: .module)
        }
    }

    public var isDemo: Bool {
        if case .demo = state { true } else { false }
    }

    /// Checks the server answers as Spotussi and accepts the token, then
    /// stores both. Nothing is saved when a check fails.
    public func pair(_ link: PairingLink) async throws(PairingError) {
        let instance = link.instance
        if instance.plainHTTPIsBlocked { throw .plainHTTPBlocked }
        let client = makeClient(instance, link.token)
        try await Self.checkIsSpotussi(client)
        try await Self.checkToken(client)
        do {
            try tokens.save(link.token)
        } catch {
            throw .other(status: nil)
        }
        defaults.set(instance.baseURL.absoluteString, forKey: Self.addressKey)
        urlCache?.removeAllCachedResponses()
        artworkCache?.removeAllCachedResponses()
        token = link.token
        state = .paired(instance, client)
    }

    /// Shows the bundled sample data. Not remembered: a relaunch returns to
    /// pairing (or to the paired instance).
    public func startDemo() {
        state = .demo
    }

    /// Forgets the instance: the token, the address and every cached answer.
    /// Also leaves demo mode.
    public func unpair() {
        try? tokens.delete()
        defaults.removeObject(forKey: Self.addressKey)
        urlCache?.removeAllCachedResponses()
        artworkCache?.removeAllCachedResponses()
        token = nil
        state = .notPaired
    }

    /// The request for an `image_url` from the API: resolved against the
    /// instance when it is relative, and carrying the token only when it
    /// goes to the instance itself, never to another host. Nil in demo
    /// mode, whose artwork DemoArtwork draws, and for anything but http(s).
    public func artworkRequest(for imageURL: String) -> URLRequest? {
        guard case .paired(let instance, _) = state,
            let url = URL(string: imageURL, relativeTo: instance.baseURL)?.absoluteURL,
            let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http"
        else { return nil }
        var request = URLRequest(url: url)
        if let token, url.hasSameOrigin(as: instance.baseURL) {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    // MARK: - Private

    private func restore() {
        guard let address = defaults.string(forKey: Self.addressKey),
            let instance = Instance(address: address),
            let token = try? tokens.token()
        else { return }
        self.token = token
        state = .paired(instance, makeClient(instance, token))
    }

    /// `/health` needs no token, so it tells a wrong address apart from a
    /// wrong token.
    private static func checkIsSpotussi(_ client: Client) async throws(PairingError) {
        let output: Operations.GetHealth.Output
        do {
            output = try await client.getHealth()
        } catch {
            switch RequestFailure(error) {
            case .unreachable: throw .unreachable
            case .plainHTTPBlocked: throw .plainHTTPBlocked
            case .cancelled: throw .cancelled
            // An answer that isn't the API's: someone else's server.
            case .tokenRejected, .other: throw .notSpotussi
            }
        }
        switch output {
        case .ok, .serviceUnavailable: return
        case .undocumented(let status, _):
            throw [502, 504].contains(status) ? .unreachable : .notSpotussi
        }
    }

    private static func checkToken(_ client: Client) async throws(PairingError) {
        let output: Operations.GetSystemStatus.Output
        do {
            output = try await client.getSystemStatus()
        } catch {
            throw PairingError(RequestFailure(error))
        }
        switch output {
        case .ok: return
        case .default(let status, _): throw PairingError(RequestFailure(apiStatus: status))
        }
    }
}

/// Why pairing failed, for the pairing screen to put into words.
public enum PairingError: Error, Equatable, Sendable {
    /// Nothing answered at that address.
    case unreachable
    /// Something answered, but not Spotussi's API.
    case notSpotussi
    /// The server refused the token (401 or 403): revoked, or mistyped.
    case tokenRejected
    /// Plain http to a public name, which App Transport Security blocks.
    /// Plain http works for IP addresses and local names only.
    case plainHTTPBlocked
    /// Pairing was cancelled before it finished.
    case cancelled
    /// Anything else. `status` is the HTTP status when the server answered.
    case other(status: Int?)

    init(_ failure: RequestFailure) {
        switch failure {
        case .unreachable: self = .unreachable
        case .plainHTTPBlocked: self = .plainHTTPBlocked
        case .tokenRejected: self = .tokenRejected
        case .cancelled: self = .cancelled
        case .other(let status): self = .other(status: status)
        }
    }
}

extension Instance {
    /// Plain http to a name App Transport Security won't allow. The app sets
    /// `NSAllowsLocalNetworking`, which covers IP addresses, `.local` names
    /// and names without a dot; any other name needs https, including a
    /// `ts.net` one.
    public var plainHTTPIsBlocked: Bool {
        guard isPlainHTTP, let host = baseURL.host()?.lowercased() else { return false }
        if host.contains(":") || !host.contains(".") || host.hasSuffix(".local") { return false }
        let parts = host.split(separator: ".", omittingEmptySubsequences: false)
        let isIPv4 = parts.count == 4 && parts.allSatisfy { UInt8($0) != nil }
        return !isIPv4
    }
}
