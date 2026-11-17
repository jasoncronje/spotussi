import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Answers API calls from the JSON recorded from `spotussi serve --mock`
/// (ios/scripts/record-fixtures.sh), one file per operation. Every range
/// shows the same sample. Lists are honest about their size: a list holds
/// what its fixture holds, `offset` and `limit` take their slice of it, and
/// there is never a next page.
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
            HTTPBody(Self.page(data, path: request.path))
        )
    }

    /// A list (`items` with a `total`) cut to the request's `offset` and
    /// `limit`, its total what the fixture holds and no next cursor. Other
    /// answers pass unchanged.
    static func page(_ data: Data, path: String?) -> Data {
        guard var object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
            let items = object["items"] as? [Any], object["total"] is Int
        else { return data }
        let query = path.flatMap { URLComponents(string: $0)?.queryItems } ?? []
        func number(_ name: String) -> Int? {
            query.first { $0.name == name }?.value.flatMap { Int($0) }
        }
        let offset = max(0, number("offset") ?? 0)
        let limit = max(0, number("limit") ?? items.count)
        // A cursor asks for the page after the first, and there is none.
        let isLaterPage = query.contains { $0.name == "cursor" }
        object["items"] = isLaterPage ? [] : Array(items.dropFirst(offset).prefix(limit))
        object["total"] = items.count
        if object["offset"] != nil { object["offset"] = offset }
        if object["limit"] != nil, let limit = number("limit") { object["limit"] = limit }
        if object.keys.contains("next_cursor") { object["next_cursor"] = NSNull() }
        return (try? JSONSerialization.data(withJSONObject: object)) ?? data
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
