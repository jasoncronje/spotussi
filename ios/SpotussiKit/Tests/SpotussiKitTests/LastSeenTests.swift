import Foundation
import HTTPTypes
import OpenAPIRuntime
import SpotussiAPI
import Synchronization
import Testing

@testable import SpotussiKit

/// The last answer to every read, served when the server can't be reached.
@Suite struct LastSeenTests {
    let cache = URLCache(memoryCapacity: 8 << 20, diskCapacity: 0)
    let storedAt = Date(timeIntervalSince1970: 1_794_871_210)
    let home = Instance(address: "https://stats.example.com")!
    let away = Instance(address: "https://other.example.com")!

    func client(_ base: HeaderStub, instance: Instance? = nil) -> Client {
        let storedAt = storedAt
        return Client(
            serverURL: (instance ?? home).apiURL,
            transport: LastSeenTransport(base: base, cache: cache, now: { storedAt }),
            middlewares: [BearerToken("spt_abcdef12")])
    }

    @Test func aFreshAnswerIsKept() async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let status = try await client(base).getSystemStatus().ok.body.json
        #expect(status.data.plays > 0)
        #expect(base.ifNoneMatch.isEmpty)
        #expect(cache.cachedResponse(for: Self.request(home, "/system/status")) != nil)
    }

    @Test func notModifiedServesTheKeptAnswer() async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        let first = try await client.getTopArtists().ok.body.json

        base.answer { _, _ in Reply.notModified }
        let second = try await client.getTopArtists().ok.body.json

        #expect(base.ifNoneMatch == ["\"v1\""])
        #expect(second.items.map(\.artist.id) == first.items.map(\.artist.id))
    }

    @Test(arguments: [URLError.Code.notConnectedToInternet, .timedOut, .cannotConnectToHost, .cannotFindHost])
    func offlineServesTheKeptAnswer(_ code: URLError.Code) async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        let first = try await client.getTopTracks().ok.body.json

        base.answer { _, _ in throw URLError(code) }
        let observer = LastSeen.Observer()
        let offline = try await LastSeen.$observer.withValue(observer) {
            try await client.getTopTracks().ok.body.json
        }

        #expect(offline.items.map(\.track.id) == first.items.map(\.track.id))
        #expect(observer.servedFromStorage)
        #expect(observer.storedAt == storedAt)
    }

    @Test func offlineWithNothingKeptFails() async {
        let base = HeaderStub { _, _ in throw URLError(.notConnectedToInternet) }
        await #expect(throws: ClientError.self) {
            _ = try await client(base).getTopTracks()
        }
    }

    @Test(arguments: [(401, "invalid_token"), (403, "forbidden"), (500, "internal")])
    func errorsAreNeverAnsweredFromStorage(_ status: Int, _ code: String) async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        _ = try await client.getSystemStatus().ok.body.json

        base.answer { _, _ in Reply.apiError(status, code: code) }
        let output = try await client.getSystemStatus()
        guard case .default(let answered, _) = output else {
            Issue.record("served \(output)")
            return
        }
        #expect(answered == status)
    }

    @Test(arguments: [502, 503, 504])
    func aGatewayAnswerIsAnsweredFromStorage(_ status: Int) async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        let fresh = try await client.getSystemStatus().ok.body.json

        base.answer { _, _ in Reply.apiError(status, code: "bad_gateway") }
        let kept = try await client.getSystemStatus().ok.body.json
        #expect(kept == fresh)
    }

    @Test func aGatewayAnswerWithNothingKeptPassesThrough() async throws {
        let base = HeaderStub { _, _ in Reply.apiError(503, code: "unavailable") }
        let output = try await client(base).getSystemStatus()
        guard case .default(let answered, _) = output else {
            Issue.record("served \(output)")
            return
        }
        #expect(answered == 503)
    }

    @Test func aTLSFailureIsNotAnsweredFromStorage() async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        _ = try await client.getSystemStatus().ok.body.json

        base.answer { _, _ in throw URLError(.serverCertificateUntrusted) }
        await #expect(throws: ClientError.self) {
            _ = try await client.getSystemStatus()
        }
    }

    @Test func oneInstanceNeverAnswersForAnother() async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        _ = try await client(base, instance: home).getTopArtists().ok.body.json

        base.answer { _, _ in throw URLError(.notConnectedToInternet) }
        await #expect(throws: ClientError.self) {
            _ = try await client(base, instance: away).getTopArtists()
        }
        #expect(base.ifNoneMatch.isEmpty)
    }

    @Test func differentQueriesAreKeptApart() async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        _ = try await client.getTopArtists(query: .init(range: .last7d)).ok.body.json

        base.answer { _, _ in throw URLError(.notConnectedToInternet) }
        await #expect(throws: ClientError.self) {
            _ = try await client.getTopArtists(query: .init(range: .last4w))
        }
    }

    @Test(arguments: ["getHealth", "getNowPlaying"])
    func liveAnswersAreNeverKept(_ operationID: String) async throws {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let client = client(base)
        try await call(operationID, client)

        base.answer { _, _ in throw URLError(.notConnectedToInternet) }
        await #expect(throws: ClientError.self) {
            try await call(operationID, client)
        }
    }

    private func call(_ operationID: String, _ client: Client) async throws {
        switch operationID {
        case "getHealth": _ = try await client.getHealth()
        default: _ = try await client.getNowPlaying()
        }
    }

    static func request(_ instance: Instance, _ path: String) -> URLRequest {
        URLRequest(url: URL(string: instance.apiURL.absoluteString + path)!)
    }
}

@MainActor @Suite struct InstanceStatusOfflineTests {
    let cache = URLCache(memoryCapacity: 8 << 20, diskCapacity: 0)
    let storedAt = Date(timeIntervalSince1970: 1_794_871_210)
    let now = Date(timeIntervalSince1970: 1_794_871_210 + 3600)

    @Test func offlineShowsTheLastStatusAsUnreachable() async {
        let base = HeaderStub { _, operationID in Reply.tagged(operationID, etag: "\"v1\"") }
        let storedAt = storedAt
        let client = Client(
            serverURL: URL(string: "https://stats.example.com/api/v1")!,
            transport: LastSeenTransport(base: base, cache: cache, now: { storedAt }))
        let model = InstanceStatus(client: client, now: { now })
        await model.refresh()
        #expect(model.failure == nil)

        base.answer { _, _ in throw URLError(.notConnectedToInternet) }
        await model.refresh()

        #expect(model.failure == .unreachable)
        #expect(model.systemStatus != nil)
        #expect(model.lastUpdated == storedAt)
        #expect(model.notices.first?.kind == .unreachable)
        #expect(model.notices.first?.message.hasPrefix("Showing what was already loaded. Last updated ") == true)
    }
}

// MARK: - Support

/// A transport that answers from a closure given the request, and records
/// the `If-None-Match` it was sent.
final class HeaderStub: ClientTransport {
    typealias Handler = @Sendable (_ request: HTTPRequest, _ operationID: String) throws -> StubTransport.Reply

    private let handler: Mutex<Handler>
    private let tags = Mutex<[String]>([])

    init(_ handler: @escaping Handler) {
        self.handler = Mutex(handler)
    }

    func answer(_ handler: @escaping Handler) {
        self.handler.withLock { $0 = handler }
    }

    var ifNoneMatch: [String] { tags.withLock { $0 } }

    func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if let tag = request.headerFields[.ifNoneMatch] {
            tags.withLock { $0.append(tag) }
        }
        return try handler.withLock { $0 }(request, operationID)
    }
}

extension Reply {
    /// A recorded answer with an ETag, as the API sends every GET.
    static func tagged(_ operationID: String, etag: String) -> StubTransport.Reply {
        (
            HTTPResponse(
                status: .ok,
                headerFields: [.contentType: "application/json", .eTag: etag, .cacheControl: "no-cache"]),
            HTTPBody(Fixtures.data(for: operationID)!)
        )
    }

    static var notModified: StubTransport.Reply {
        (HTTPResponse(status: .notModified, headerFields: [:]), nil)
    }
}
