import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession
import SpotussiAPI

extension Client {
    /// A client for a paired instance. Every request carries the read-only
    /// token; answers are kept in `cache`, revalidated with the API's ETags
    /// and served from there when the server can't be reached.
    public static func live(
        _ instance: Instance, token: String, session: URLSession = .spotussi, cache: URLCache = .spotussiAPI
    ) -> Client {
        Client(
            serverURL: instance.apiURL,
            transport: LastSeenTransport(
                base: URLSessionTransport(configuration: .init(session: session)), cache: cache),
            middlewares: [BearerToken(token)]
        )
    }

    /// A client that answers from the sample data bundled with the app:
    /// demo mode, previews and tests.
    public static var demo: Client {
        Client(
            serverURL: URL(string: "https://demo.invalid/api/v1")!,
            transport: DemoTransport()
        )
    }
}

extension URLSession {
    /// One session for API calls. It keeps no cache of its own:
    /// `LastSeenTransport` keeps the answers and asks with their ETags.
    public static let spotussi: URLSession = {
        let config = URLSessionConfiguration.default
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.timeoutIntervalForRequest = 20
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()
}

/// Adds `Authorization: Bearer <token>` to every request.
struct BearerToken: ClientMiddleware {
    private let header: String

    init(_ token: String) { header = "Bearer \(token)" }

    func intercept(
        _ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        request.headerFields[.authorization] = header
        return try await next(request, body, baseURL)
    }
}
