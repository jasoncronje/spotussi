import Foundation
import HTTPTypes
import OpenAPIRuntime
import SpotussiAPI
import Synchronization

@testable import SpotussiKit

/// A transport that answers from a closure and records what it was asked.
final class StubTransport: ClientTransport {
    typealias Reply = (HTTPResponse, HTTPBody?)
    typealias Handler = @Sendable (_ operationID: String) throws -> Reply

    private let handler: Mutex<Handler>
    private let requests = Mutex<[(operationID: String, authorization: String?)]>([])

    init(_ handler: @escaping Handler) {
        self.handler = Mutex(handler)
    }

    /// Changes the answers from here on.
    func answer(_ handler: @escaping Handler) {
        self.handler.withLock { $0 = handler }
    }

    var operations: [String] { requests.withLock { $0.map(\.operationID) } }

    func authorization(for operationID: String) -> String? {
        requests.withLock { $0.last { $0.operationID == operationID }?.authorization }
    }

    func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        requests.withLock {
            $0.append((operationID, request.headerFields[.authorization]))
        }
        return try handler.withLock { $0 }(operationID)
    }

    func client(_ instance: Instance, token: String) -> Client {
        Client(serverURL: instance.apiURL, transport: self, middlewares: [BearerToken(token)])
    }

    /// A client with no token, for models that take one ready-made.
    var apiClient: Client {
        Client(serverURL: URL(string: "https://stats.example.com/api/v1")!, transport: self)
    }
}

enum Reply {
    static func fixture(_ operationID: String) -> StubTransport.Reply {
        json(Fixtures.data(for: operationID)!)
    }

    static func json(_ data: Data, status: Int = 200) -> StubTransport.Reply {
        (
            HTTPResponse(status: .init(code: status), headerFields: [.contentType: "application/json"]),
            HTTPBody(data)
        )
    }

    /// An error in the API's own shape.
    static func apiError(_ status: Int, code: String) -> StubTransport.Reply {
        json(Data(#"{"error":{"code":"\#(code)","message":"No."}}"#.utf8), status: status)
    }

    /// A page from something that isn't the API: a proxy, a login page.
    static func html(_ status: Int) -> StubTransport.Reply {
        (
            HTTPResponse(status: .init(code: status), headerFields: [.contentType: "text/html"]),
            HTTPBody("<html><body>Hello</body></html>")
        )
    }
}

/// A `UserDefaults` suite of its own, removed when the test is done.
final class TestDefaults {
    let name = "SpotussiKitTests.\(UUID().uuidString)"
    let defaults: UserDefaults

    init() {
        defaults = UserDefaults(suiteName: name)!
    }

    deinit {
        defaults.removePersistentDomain(forName: name)
    }
}

func fixtureStatus() throws -> Components.Schemas.SystemStatus {
    try JSONDecoder().decode(
        Components.Schemas.SystemStatus.self, from: Fixtures.data(for: "getSystemStatus")!)
}
