import Foundation
import Observation
import SpotussiAPI

/// The instance's state as the notices above every screen show it: whether
/// the server answers, whether Spotify needs reconnecting, how far catalog
/// lookups are. Read-only; the fixes all live on the web.
@MainActor @Observable
public final class InstanceStatus {
    public enum Failure: Equatable, Sendable {
        /// No answer: the server is down or only reachable at home.
        case unreachable
        /// The server refused the token, so it was revoked.
        case tokenRejected
        /// Any other failed refresh.
        case other
    }

    /// The last status that loaded; kept when a later refresh fails.
    public private(set) var systemStatus: Components.Schemas.SystemStatus?
    /// The last Spotify connection that loaded (account name, scopes).
    public private(set) var spotifyConnection: Components.Schemas.SpotifyConnection?
    /// The server's version from `/health`, once it has answered.
    public private(set) var serverVersion: String?
    /// When `systemStatus` was loaded.
    public private(set) var lastUpdated: Date?
    /// Why the latest refresh failed; nil once one succeeds.
    public private(set) var failure: Failure?
    public private(set) var isRefreshing = false

    @ObservationIgnored private let client: Client
    @ObservationIgnored private let now: () -> Date

    public init(client: Client, now: @escaping () -> Date = Date.init) {
        self.client = client
        self.now = now
    }

    public var isUnreachable: Bool { failure == .unreachable }

    /// What to show, most pressing first.
    public var notices: [StatusNotice] {
        let now = self.now()
        var notices: [StatusNotice] = []
        switch failure {
        case .unreachable: notices.append(.unreachable(lastUpdated: lastUpdated, now: now))
        case .tokenRejected: notices.append(.tokenRejected)
        case .other, nil: break
        }
        if let systemStatus {
            notices += StatusNotice.notices(for: systemStatus, now: now)
        }
        return notices
    }

    /// Loads the status and Spotify connection together. On failure the last
    /// good answers stay; a cancelled refresh changes nothing.
    public func refresh() async {
        isRefreshing = true
        defer { isRefreshing = false }
        do {
            // The version is for the Settings sheet only, so its failure
            // doesn't count as the refresh failing.
            async let health = try? client.getHealth().ok.body.json
            async let status = client.getSystemStatus()
            async let connection = client.getSpotifyConnection()
            let (statusOutput, connectionOutput) = try await (status, connection)
            if let version = (await health)?.version { serverVersion = version }
            let newStatus: Components.Schemas.SystemStatus
            let newConnection: Components.Schemas.SpotifyConnection
            switch statusOutput {
            case .ok(let ok): newStatus = try ok.body.json
            case .default(let code, _): return fail(RequestFailure(apiStatus: code))
            }
            switch connectionOutput {
            case .ok(let ok): newConnection = try ok.body.json
            case .default(let code, _): return fail(RequestFailure(apiStatus: code))
            }
            systemStatus = newStatus
            spotifyConnection = newConnection
            lastUpdated = now()
            failure = nil
        } catch {
            fail(RequestFailure(error))
        }
    }

    private func fail(_ requestFailure: RequestFailure) {
        switch requestFailure {
        case .cancelled: return
        case .unreachable: failure = .unreachable
        case .tokenRejected: failure = .tokenRejected
        case .plainHTTPBlocked, .other: failure = .other
        }
    }
}

/// One line of instance status, ready to show and to read aloud.
public struct StatusNotice: Identifiable, Hashable, Sendable {
    public enum Kind: String, Hashable, Sendable {
        case unreachable
        case tokenRejected
        case reconnectSpotify
        case spotifyNotConnected
        case catalogProgress
    }

    public let kind: Kind
    /// The lead sentence, shown in bold on the web.
    public let title: String
    public let message: String
    /// For the catalog notice: how far lookups are, for the lamp row.
    public let progress: CatalogProgress?

    public var id: Kind { kind }

    /// Something has stopped working (the web's red and signal banners), as
    /// opposed to information.
    public var isAlert: Bool {
        switch kind {
        case .unreachable, .tokenRejected, .reconnectSpotify: true
        case .spotifyNotConnected, .catalogProgress: false
        }
    }

    /// Title and message as one sentence pair for VoiceOver.
    public var accessibilityLabel: String { "\(title) \(message)" }
}

// MARK: - Reduction

extension StatusNotice {
    /// The notices the web shell derives from `/system/status`
    /// (status-banners.svelte and catalog-strip.svelte).
    public static func notices(
        for status: Components.Schemas.SystemStatus, now: Date
    ) -> [StatusNotice] {
        var notices: [StatusNotice] = []
        switch status.spotify.state {
        case .reconnectRequired:
            notices.append(.reconnectSpotify(lastPlayAt: status.capture.lastPlayAt.map(date), now: now))
        case .notConfigured where status.data.rows > 0:
            notices.append(.spotifyNotConnected)
        case .notConfigured, .connected:
            break
        }
        if let progress = CatalogProgress(status.catalog), progress.isWorthShowing {
            notices.append(.catalog(progress, catalog: status.catalog, now: now))
        }
        return notices
    }

    static func unreachable(lastUpdated: Date?, now: Date) -> StatusNotice {
        let shown = String(localized: "Showing what was already loaded.", bundle: .module)
        let message =
            if let lastUpdated {
                shown + " "
                    + String(
                        localized: "Last updated \(RelativeTime.ago(lastUpdated, now: now)).",
                        bundle: .module)
            } else {
                shown
            }
        return StatusNotice(
            kind: .unreachable,
            title: String(localized: "Can’t reach the server.", bundle: .module),
            message: message, progress: nil)
    }

    static let tokenRejected = StatusNotice(
        kind: .tokenRejected,
        title: String(localized: "This iPhone’s access was revoked.", bundle: .module),
        message: String(
            localized: "Pair it again from Settings, API & MCP on the web.", bundle: .module),
        progress: nil)

    static func reconnectSpotify(lastPlayAt: Date?, now: Date) -> StatusNotice {
        let message =
            if let lastPlayAt {
                String(
                    localized:
                        "Live capture stopped; last play caught \(RelativeTime.ago(lastPlayAt, now: now)). Your history is safe.",
                    bundle: .module)
            } else {
                String(localized: "Live capture stopped. Your history is safe.", bundle: .module)
            }
        return StatusNotice(
            kind: .reconnectSpotify,
            title: String(localized: "Spotify needs reconnecting.", bundle: .module),
            message: message, progress: nil)
    }

    static let spotifyNotConnected = StatusNotice(
        kind: .spotifyNotConnected,
        title: String(localized: "Spotify isn’t connected.", bundle: .module),
        message: String(localized: "New plays aren’t being captured.", bundle: .module),
        progress: nil)

    static func catalog(
        _ progress: CatalogProgress, catalog: Components.Schemas.SystemStatus.CatalogPayload,
        now: Date
    ) -> StatusNotice {
        var message =
            switch catalog.state {
            case .quotaExceeded:
                if let resumesAt = catalog.resumesAt {
                    String(
                        localized:
                            "Spotify’s daily lookup limit is reached; lookups resume \(RelativeTime.ago(date(resumesAt), now: now)).",
                        bundle: .module)
                } else {
                    String(localized: "Spotify’s daily lookup limit is reached.", bundle: .module)
                }
            case .backingOff:
                String(localized: "Lookups are retrying after errors.", bundle: .module)
            case .waiting:
                String(localized: "Lookups wait for the Spotify connection.", bundle: .module)
            default:
                if let etaAt = catalog.etaAt {
                    String(
                        localized:
                            "Artwork, lengths and genres fill in as it runs, done around \(date(etaAt).formatted(date: .omitted, time: .shortened)).",
                        bundle: .module)
                } else {
                    String(
                        localized: "Artwork, lengths and genres fill in as it runs.",
                        bundle: .module)
                }
            }
        if progress.provisional > 0 {
            message += " "
            message += String(
                localized: "Provisional details cover \(progress.provisional.formatted()) more.",
                bundle: .module)
        }
        return StatusNotice(
            kind: .catalogProgress,
            title: String(localized: "Track details \(progress.percent)% looked up.", bundle: .module),
            message: message, progress: progress)
    }
}

/// How far Spotify lookups of the catalog are (D9, D27).
public struct CatalogProgress: Hashable, Sendable {
    /// Tracks with details from Spotify.
    public let lookedUp: Int
    public let total: Int
    /// Tracks without Spotify details that a fallback source matched.
    public let provisional: Int
    /// Lookups are running or held up, rather than done or switched off.
    public let isActive: Bool

    /// Nil when there is no catalog to look up.
    init?(_ catalog: Components.Schemas.SystemStatus.CatalogPayload) {
        guard catalog.tracksTotal > 0 else { return nil }
        total = catalog.tracksTotal
        lookedUp = catalog.tracksHydrated
        // A track can match several sources; take the best. MusicBrainz and
        // ListenBrainz add genres, not track details, so they don't count.
        let matched = catalog.fallbacks
            .filter { $0.source != .musicbrainz && $0.source != .listenbrainz }
            .map(\.matched)
            .max() ?? 0
        provisional = min(max(0, total - lookedUp), max(0, matched))
        isActive = [.hydrating, .waiting, .backingOff, .quotaExceeded].contains(catalog.state)
    }

    /// Share of tracks with Spotify details, 0...1.
    public var share: Double { Double(lookedUp) / Double(total) }
    /// Share of tracks with provisional details only.
    public var provisionalShare: Double { Double(provisional) / Double(total) }
    /// Whole percent looked up, rounded down so 99.6% never reads as 100%.
    public var percent: Int { Int((share * 100).rounded(.down)) }

    /// Shown until nearly all listening has details.
    var isWorthShowing: Bool { isActive && share < 0.97 }

    /// How many of `count` lamps to light fully and dimly, as the web's
    /// catalog strip does with 24.
    public func lamps(count: Int = 24) -> (lit: Int, dim: Int) {
        let lit = Int((share * Double(count)).rounded())
        let dim = min(count - lit, Int((provisionalShare * Double(count)).rounded()))
        return (lit, dim)
    }
}

private func date(_ epochMilliseconds: Components.Schemas.EpochMs) -> Date {
    Date(timeIntervalSince1970: Double(epochMilliseconds) / 1000)
}
