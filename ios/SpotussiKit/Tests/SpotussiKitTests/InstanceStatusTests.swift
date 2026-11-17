import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

@Suite struct StatusNoticeTests {
    /// Just after the fixture's last poll.
    let now = Date(timeIntervalSince1970: 1_794_871_210)

    func notices(_ change: (inout Components.Schemas.SystemStatus) -> Void = { _ in }) throws
        -> [StatusNotice]
    {
        var status = try fixtureStatus()
        change(&status)
        return StatusNotice.notices(for: status, now: now)
    }

    /// The recorded instance is connected and 98% looked up: nothing to say.
    @Test func theRecordedStatusIsQuiet() throws {
        #expect(try notices().isEmpty)
    }

    @Test func spotifyNeedsReconnecting() throws {
        let notices = try notices {
            $0.spotify.state = .reconnectRequired
            $0.capture.lastPlayAt = nil
        }
        let notice = try #require(notices.first)
        #expect(notices.map(\.kind) == [.reconnectSpotify])
        #expect(notice.isAlert)
        #expect(notice.title == "Spotify needs reconnecting.")
        #expect(notice.message == "Live capture stopped. Your history is safe.")
        #expect(notice.accessibilityLabel == "\(notice.title) \(notice.message)")
    }

    @Test func reconnectingSaysWhenTheLastPlayWasCaught() throws {
        let notice = try #require(try notices { $0.spotify.state = .reconnectRequired }.first)
        #expect(notice.message.hasPrefix("Live capture stopped; last play caught "))
        #expect(notice.message.hasSuffix(". Your history is safe."))
    }

    @Test func spotifyNotConnectedOnlyMattersWithHistory() throws {
        let withHistory = try notices { $0.spotify.state = .notConfigured }
        #expect(withHistory.map(\.kind) == [.spotifyNotConnected])
        #expect(withHistory.first?.isAlert == false)
        #expect(withHistory.first?.message == "New plays aren’t being captured.")

        let empty = try notices {
            $0.spotify.state = .notConfigured
            $0.data.rows = 0
        }
        #expect(empty.isEmpty)
    }

    @Test func catalogProgress() throws {
        let notices = try notices {
            $0.catalog.tracksTotal = 1000
            $0.catalog.tracksHydrated = 405
            $0.catalog.etaAt = nil
            $0.catalog.fallbacks[0].matched = 40  // oembed
            $0.catalog.fallbacks[1].matched = 90  // deezer
            $0.catalog.fallbacks[3].matched = 900  // musicbrainz: genres only
        }
        let notice = try #require(notices.first)
        #expect(notices.map(\.kind) == [.catalogProgress])
        #expect(notice.title == "Track details 40% looked up.")
        #expect(
            notice.message
                == "Artwork, lengths and genres fill in as it runs. Provisional details cover 90 more.")
        let progress = try #require(notice.progress)
        #expect(progress.provisional == 90)
        #expect(progress.lamps().lit == 10)  // round(0.405 × 24)
        #expect(progress.lamps().dim == 2)  // round(0.09 × 24)
    }

    @Test func catalogProgressSaysWhyItIsSlow() throws {
        func message(_ state: Components.Schemas.SystemStatus.CatalogPayload.StatePayload) throws -> String? {
            try notices {
                $0.catalog.state = state
                $0.catalog.tracksHydrated = 100
                $0.catalog.resumesAt = nil
            }.first?.message
        }
        #expect(try message(.quotaExceeded) == "Spotify’s daily lookup limit is reached.")
        #expect(try message(.backingOff) == "Lookups are retrying after errors.")
        #expect(try message(.waiting) == "Lookups wait for the Spotify connection.")
        #expect(try message(.idle) == nil)
        #expect(try message(.disabled) == nil)
    }

    @Test func catalogProgressHidesOnceNearlyDone() throws {
        let at97 = try notices {
            $0.catalog.tracksTotal = 100
            $0.catalog.tracksHydrated = 97
        }
        #expect(at97.isEmpty)
        let none = try notices { $0.catalog.tracksTotal = 0 }
        #expect(none.isEmpty)
    }

    @Test func noticesComeMostPressingFirst() throws {
        let notices = try notices {
            $0.spotify.state = .reconnectRequired
            $0.catalog.tracksHydrated = 10
        }
        #expect(notices.map(\.kind) == [.reconnectSpotify, .catalogProgress])
    }

    @Test(arguments: [
        (-20.0, "just now"),
        (20.0, "just now"),
    ])
    func agoMatchesTheWebForTheNearPast(_ offset: TimeInterval, _ expected: String) {
        #expect(RelativeTime.ago(now.addingTimeInterval(offset), now: now) == expected)
    }
}

@MainActor @Suite struct InstanceStatusTests {
    let now = Date(timeIntervalSince1970: 1_794_871_210)

    @Test func loadsTheDemoInstance() async {
        let model = InstanceStatus(client: .demo, now: { now })
        await model.refresh()
        #expect(model.systemStatus?.data.plays == 43762)
        #expect(model.spotifyConnection?.state == .notConfigured)
        #expect(model.lastUpdated == now)
        #expect(model.serverVersion == "dev")
        #expect(model.failure == nil)
        #expect(!model.isRefreshing)
        #expect(model.notices.isEmpty)
    }

    @Test func aMissingVersionDoesNotFailTheRefresh() async {
        let transport = StubTransport { operationID in
            operationID == "getHealth" ? Reply.html(404) : Reply.fixture(operationID)
        }
        let model = InstanceStatus(client: transport.apiClient, now: { now })
        await model.refresh()
        #expect(model.failure == nil)
        #expect(model.serverVersion == nil)
        #expect(model.systemStatus != nil)
    }

    @Test func keepsTheLastGoodStatusWhenTheServerGoesAway() async throws {
        let transport = StubTransport { operationID in
            var status = try fixtureStatus()
            status.spotify.state = .reconnectRequired
            return operationID == "getSystemStatus"
                ? Reply.json(try JSONEncoder().encode(status))
                : Reply.fixture(operationID)
        }
        var clock = now
        let model = InstanceStatus(client: transport.apiClient, now: { clock })
        await model.refresh()

        clock = now.addingTimeInterval(600)
        transport.answer { _ in throw URLError(.notConnectedToInternet) }
        await model.refresh()

        #expect(model.isUnreachable)
        #expect(model.lastUpdated == now)
        #expect(model.systemStatus != nil)
        #expect(model.notices.map(\.kind) == [.unreachable, .reconnectSpotify])
        let notice = try #require(model.notices.first)
        #expect(notice.title == "Can’t reach the server.")
        #expect(notice.message.hasPrefix("Showing what was already loaded. Last updated "))

        transport.answer { Reply.fixture($0) }
        await model.refresh()
        #expect(model.failure == nil)
        #expect(model.lastUpdated == clock)
    }

    @Test func unreachableBeforeAnythingLoaded() async throws {
        let transport = StubTransport { _ in Reply.html(502) }
        let model = InstanceStatus(client: transport.apiClient, now: { now })
        await model.refresh()
        let notice = try #require(model.notices.first)
        #expect(model.notices.count == 1)
        #expect(notice.kind == .unreachable)
        #expect(notice.message == "Showing what was already loaded.")
    }

    @Test func aRevokedToken() async {
        let transport = StubTransport { _ in Reply.apiError(401, code: "invalid_token") }
        let model = InstanceStatus(client: transport.apiClient, now: { now })
        await model.refresh()
        #expect(model.failure == .tokenRejected)
        #expect(model.notices.map(\.kind) == [.tokenRejected])
    }

    @Test func otherFailuresShowNoNotice() async {
        let transport = StubTransport { _ in Reply.apiError(500, code: "internal") }
        let model = InstanceStatus(client: transport.apiClient, now: { now })
        await model.refresh()
        #expect(model.failure == .other)
        #expect(model.notices.isEmpty)
    }
}
