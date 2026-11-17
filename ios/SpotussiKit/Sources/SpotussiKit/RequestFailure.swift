import Foundation
import OpenAPIRuntime

/// What a failed API call means for the person holding the phone. Mirrors
/// `unreachable()` in web/src/lib/state/link.svelte.ts.
enum RequestFailure: Equatable, Sendable {
    /// No answer, or a gateway in front of the server saying it is down.
    case unreachable
    /// App Transport Security refused plain http to this host.
    case plainHTTPBlocked
    /// The server refused the token (revoked, or never valid).
    case tokenRejected
    /// The request was cancelled (the screen went away); not a failure.
    case cancelled
    /// The server answered, but not with what was asked for.
    case other(status: Int?)

    /// Gateway answers that mean the server itself isn't there. A 404 with no
    /// API error body is the reverse proxy having no route: Traefik drops the
    /// router of a stopped container.
    private static let gateway: Set<Int> = [404, 502, 503, 504]

    init(_ error: any Error) {
        if error is CancellationError {
            self = .cancelled
            return
        }
        guard let clientError = error as? ClientError else {
            self = Self.transport(error)
            return
        }
        guard let response = clientError.response else {
            self = Self.transport(clientError.underlyingError)
            return
        }
        // The server answered, but the body didn't decode as the API's: an
        // HTML page from a proxy, a login redirect, another app.
        let status = response.status.code
        if Self.gateway.contains(status) {
            self = .unreachable
        } else {
            self = Self(apiStatus: status)
        }
    }

    /// An error response with the API's own body (it carries a code).
    init(apiStatus status: Int) {
        self = status == 401 || status == 403 ? .tokenRejected : .other(status: status)
    }

    private static func transport(_ error: any Error) -> RequestFailure {
        if error is CancellationError { return .cancelled }
        guard let urlError = error as? URLError else { return .other(status: nil) }
        switch urlError.code {
        case .cancelled: return .cancelled
        case .appTransportSecurityRequiresSecureConnection: return .plainHTTPBlocked
        default: return .unreachable
        }
    }
}
