import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Answers API calls from the JSON recorded from `spotussi serve --mock`
/// (ios/scripts/record-fixtures.sh), one file per operation. Parameters are
/// ignored: every range shows the same sample, which is enough for a tour
/// of the app and for tests of decoding.
public struct DemoTransport: ClientTransport {
    public init() {}

    public func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        guard let data = Fixtures.data(for: operationID) else {
            let problem = #"{"error":{"code":"not_found","message":"Not part of the demo."}}"#
            return (
                HTTPResponse(status: .notFound, headerFields: [.contentType: "application/json"]),
                HTTPBody(problem)
            )
        }
        return (
            HTTPResponse(status: .ok, headerFields: [.contentType: "application/json"]),
            HTTPBody(data)
        )
    }
}

/// The bundled sample responses.
public enum Fixtures {
    /// Operation IDs that have a recorded response.
    public static var operations: [String] {
        let urls = Bundle.module.urls(forResourcesWithExtension: "json", subdirectory: "Fixtures")
        return (urls ?? []).map { $0.deletingPathExtension().lastPathComponent }.sorted()
    }

    public static func data(for operationID: String) -> Data? {
        guard
            let url = Bundle.module.url(
                forResource: operationID, withExtension: "json", subdirectory: "Fixtures")
        else { return nil }
        return try? Data(contentsOf: url)
    }
}
