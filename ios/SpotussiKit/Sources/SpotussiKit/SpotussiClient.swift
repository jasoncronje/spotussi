import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession
import SpotussiAPI

extension Client {
    /// A client for a paired instance. Every request carries the read-only
    /// token; responses are cached by `URLCache` and revalidated with the
    /// API's ETags (it answers every GET with `Cache-Control: no-cache`).
    public static func live(
        _ instance: Instance, token: String, session: URLSession = .spotussi
    ) -> Client {
        Client(
            serverURL: instance.apiURL,
            transport: URLSessionTransport(configuration: .init(session: session)),
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
    /// One session for API calls, with a disk cache so the last answer for
    /// each request survives a relaunch and a server that can't be reached.
    public static let spotussi: URLSession = {
        let config = URLSessionConfiguration.default
        config.urlCache = URLCache(
            memoryCapacity: 8 << 20, diskCapacity: 64 << 20,
            directory: URL.cachesDirectory.appending(path: "api"))
        config.requestCachePolicy = .useProtocolCachePolicy
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
