import Foundation
import Testing

@testable import SpotussiKit

@MainActor @Suite struct SessionTests {
    let store = InMemoryTokenStore()
    let suite = TestDefaults()
    let cache = URLCache(memoryCapacity: 1 << 20, diskCapacity: 0)
    let artworkCache = URLCache(memoryCapacity: 1 << 20, diskCapacity: 0)
    let link = try! PairingLink(address: "https://stats.example.com", token: "spt_abcdef12")

    func session(_ transport: StubTransport) -> Session {
        Session(
            tokens: store, defaults: suite.defaults, urlCache: cache, artworkCache: artworkCache,
            makeClient: { transport.client($0, token: $1) })
    }

    /// The server's own answers: `/health` and `/system/status`.
    static let healthy: StubTransport.Handler = { operationID in
        Reply.fixture(operationID)
    }

    // MARK: - Pairing

    @Test func pairsAfterCheckingTheServerAndTheToken() async throws {
        let transport = StubTransport(Self.healthy)
        let session = session(transport)
        fillCache()

        try await session.pair(link)

        #expect(transport.operations == ["getHealth", "getSystemStatus"])
        #expect(transport.authorization(for: "getSystemStatus") == "Bearer spt_abcdef12")
        #expect(session.instance == link.instance)
        #expect(session.displayName == "stats.example.com")
        #expect(session.client != nil)
        #expect(store.token() == "spt_abcdef12")
        #expect(suite.defaults.string(forKey: "instance.address") == "https://stats.example.com")
        #expect(cache.cachedResponse(for: Self.cachedRequest) == nil)
    }

    @Test(arguments: [URLError.Code.cannotConnectToHost, .timedOut, .cannotFindHost])
    func nothingAnswering(_ code: URLError.Code) async {
        let transport = StubTransport { _ in throw URLError(code) }
        await expectFailure(.unreachable, transport)
    }

    @Test func aGatewayWithNothingBehindIt() async {
        let transport = StubTransport { _ in Reply.html(502) }
        await expectFailure(.unreachable, transport)
    }

    @Test(arguments: [200, 404])
    func somethingElseAnswering(_ status: Int) async {
        let transport = StubTransport { _ in Reply.html(status) }
        await expectFailure(.notSpotussi, transport)
    }

    @Test func otherJSONAtTheAddress() async {
        let transport = StubTransport { _ in Reply.json(Data(#"{"hello":"world"}"#.utf8)) }
        await expectFailure(.notSpotussi, transport)
    }

    @Test(arguments: [(401, "invalid_token"), (403, "forbidden")])
    func theTokenRefused(_ status: Int, _ code: String) async {
        let transport = StubTransport { operationID in
            operationID == "getHealth" ? Reply.fixture(operationID) : Reply.apiError(status, code: code)
        }
        await expectFailure(.tokenRejected, transport)
    }

    @Test func aServerErrorAfterTheHealthCheck() async {
        let transport = StubTransport { operationID in
            operationID == "getHealth" ? Reply.fixture(operationID) : Reply.apiError(500, code: "internal")
        }
        await expectFailure(.other(status: 500), transport)
    }

    @Test func plainHTTPToAPublicNameIsRefusedBeforeAsking() async throws {
        let transport = StubTransport(Self.healthy)
        let link = try PairingLink(address: "http://stats.example.com", token: "spt_abcdef12")
        await expectFailure(.plainHTTPBlocked, transport, link: link)
        #expect(transport.operations.isEmpty)
    }

    @Test func plainHTTPRefusedByTheSystem() async {
        let transport = StubTransport { _ in
            throw URLError(.appTransportSecurityRequiresSecureConnection)
        }
        await expectFailure(.plainHTTPBlocked, transport)
    }

    @Test(arguments: [
        ("http://192.168.1.20:8080", false),
        ("http://100.101.102.103", false),
        ("http://nas.local", false),
        ("http://nas", false),
        ("http://[fd7a:115c::1]:8080", false),
        ("https://stats.example.com", false),
        ("http://stats.example.com", true),
        ("http://box.tail1234.ts.net", true),
    ])
    func whichPlainHTTPAddressesAreBlocked(_ address: String, _ blocked: Bool) {
        #expect(Instance(address: address)!.plainHTTPIsBlocked == blocked)
    }

    // MARK: - Restore, demo, unpair

    @Test func restoresTheStoredInstance() {
        store.save("spt_abcdef12")
        suite.defaults.set("http://10.0.0.5:8080", forKey: "instance.address")
        let session = session(StubTransport(Self.healthy))
        #expect(session.instance?.baseURL.absoluteString == "http://10.0.0.5:8080")
        #expect(session.displayName == "10.0.0.5:8080")
    }

    @Test func anAddressWithoutATokenIsNotPaired() {
        suite.defaults.set("https://stats.example.com", forKey: "instance.address")
        let session = session(StubTransport(Self.healthy))
        #expect(session.instance == nil)
        #expect(session.client == nil)
    }

    @Test func unpairingForgetsEverything() async throws {
        let session = session(StubTransport(Self.healthy))
        try await session.pair(link)
        fillCache()

        session.unpair()

        guard case .notPaired = session.state else {
            Issue.record("still \(session.state)")
            return
        }
        #expect(store.token() == nil)
        #expect(suite.defaults.string(forKey: "instance.address") == nil)
        #expect(cache.cachedResponse(for: Self.cachedRequest) == nil)
        #expect(self.session(StubTransport(Self.healthy)).instance == nil)
    }

    @Test func demoAndBack() async throws {
        let session = session(StubTransport(Self.healthy))
        session.startDemo()
        #expect(session.isDemo)
        #expect(session.displayName == "Demo")
        let status = try await session.client!.getSystemStatus().ok.body.json
        #expect(status.data.plays > 0)

        session.unpair()
        #expect(!session.isDemo)
        #expect(session.client == nil)
    }

    // MARK: - Helpers

    private func expectFailure(
        _ expected: PairingError, _ transport: StubTransport, link: PairingLink? = nil,
        sourceLocation: SourceLocation = #_sourceLocation
    ) async {
        let session = session(transport)
        await #expect(throws: expected, sourceLocation: sourceLocation) {
            try await session.pair(link ?? self.link)
        }
        #expect(session.instance == nil, sourceLocation: sourceLocation)
        #expect(store.token() == nil, sourceLocation: sourceLocation)
        #expect(
            suite.defaults.string(forKey: "instance.address") == nil,
            sourceLocation: sourceLocation)
    }

    // MARK: - Artwork

    @Test func artworkFromTheInstanceCarriesTheToken() async throws {
        let session = session(StubTransport(Self.healthy))
        try await session.pair(link)

        let relative = try #require(session.artworkRequest(for: "/_mock/img/album/a.svg"))
        #expect(relative.url?.absoluteString == "https://stats.example.com/_mock/img/album/a.svg")
        #expect(relative.value(forHTTPHeaderField: "Authorization") == "Bearer spt_abcdef12")

        let absolute = try #require(session.artworkRequest(for: "https://STATS.example.com:443/img/b.jpg"))
        #expect(absolute.value(forHTTPHeaderField: "Authorization") == "Bearer spt_abcdef12")
    }

    @Test(arguments: [
        "https://i.scdn.co/image/ab67616d0000b273",
        "//i.scdn.co/image/ab67616d0000b273",
        "http://stats.example.com/img/b.jpg",
        "https://stats.example.com:8443/img/b.jpg",
    ])
    func artworkElsewhereGoesWithoutTheToken(_ address: String) async throws {
        let session = session(StubTransport(Self.healthy))
        try await session.pair(link)
        let request = try #require(session.artworkRequest(for: address))
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test func noArtworkWithoutAnInstance() async throws {
        let session = session(StubTransport(Self.healthy))
        #expect(session.artworkRequest(for: "/img/a.jpg") == nil)
        session.startDemo()
        #expect(session.artworkRequest(for: "/img/a.jpg") == nil)
        try await session.pair(link)
        #expect(session.artworkRequest(for: "data:image/png;base64,AAAA") == nil)

        let request = URLRequest(url: URL(string: "https://stats.example.com/img/a.jpg")!)
        artworkCache.storeCachedResponse(
            CachedURLResponse(
                response: HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!,
                data: Data(repeating: 1, count: 64)),
            for: request)
        session.unpair()
        #expect(session.artworkRequest(for: "/img/a.jpg") == nil)
        #expect(artworkCache.cachedResponse(for: request) == nil)
    }

    private static let cachedRequest = URLRequest(
        url: URL(string: "https://stats.example.com/api/v1/system/status")!)

    private func fillCache() {
        cache.storeCachedResponse(
            CachedURLResponse(
                response: HTTPURLResponse(
                    url: Self.cachedRequest.url!, statusCode: 200, httpVersion: nil,
                    headerFields: nil)!,
                data: Data(repeating: 1, count: 1024)),
            for: Self.cachedRequest)
        #expect(cache.cachedResponse(for: Self.cachedRequest) != nil)
    }
}
