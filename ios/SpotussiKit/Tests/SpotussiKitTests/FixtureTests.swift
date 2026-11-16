import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

/// Every recorded response decodes into the generated types, through the
/// same client the app uses in demo mode. A failure here means the spec,
/// the generated client and the fixtures have drifted apart.
@Suite struct FixtureTests {
    let client = Client.demo

    @Test func everyOperationTheAppCallsHasAFixture() {
        #expect(Fixtures.operations.count == 33)
    }

    @Test func status() async throws {
        #expect(try await client.getHealth().ok.body.json.status == .ok)
        _ = try await client.getSystemStatus().ok.body.json
        _ = try await client.getSpotifyConnection().ok.body.json
        _ = try await client.getSettings().ok.body.json
        _ = try await client.getNowPlaying().ok.body.json
    }

    @Test func overview() async throws {
        let summary = try await client.getOverviewSummary().ok.body.json
        #expect(summary.current.plays > 0)
        #expect(try await client.getTimeline().ok.body.json.points.isEmpty == false)
        #expect(try await client.getClock().ok.body.json.hours.count == 24)
    }

    @Test func topLists() async throws {
        #expect(try await client.getTopTracks().ok.body.json.items.count == 25)
        #expect(try await client.getTopArtists().ok.body.json.items.count == 25)
        #expect(try await client.getTopAlbums().ok.body.json.items.count == 25)
        #expect(try await client.getTopGenres().ok.body.json.items.count == 25)
    }

    @Test func stats() async throws {
        _ = try await client.getHourlyTop().ok.body.json
        _ = try await client.getArtistsOverTime().ok.body.json
        _ = try await client.getGenresOverTime().ok.body.json
        _ = try await client.getReleaseYears().ok.body.json
        _ = try await client.getArtistsPerTrack().ok.body.json
    }

    @Test func entityPages() async throws {
        _ = try await client.getArtist(path: .init(id: "demo")).ok.body.json
        _ = try await client.getAlbum(path: .init(id: "demo")).ok.body.json
        _ = try await client.getTrack(path: .init(id: "demo")).ok.body.json
        _ = try await client.getGenre(path: .init(id: "demo")).ok.body.json
    }

    @Test func historyAndSessions() async throws {
        #expect(try await client.getHistory().ok.body.json.items.count == 25)
        _ = try await client.getDevices().ok.body.json
        _ = try await client.getContexts().ok.body.json
        let sessions = try await client.getSessions().ok.body.json
        #expect(sessions.items.isEmpty == false)
        _ = try await client.getSession(path: .init(id: "demo")).ok.body.json
    }

    @Test func library() async throws {
        _ = try await client.getLibrarySummary().ok.body.json
        _ = try await client.getLibraryTracks().ok.body.json
        _ = try await client.getLibraryAlbums().ok.body.json
        _ = try await client.getLibraryPlaylists().ok.body.json
        _ = try await client.getLibraryPlaylist(path: .init(id: "demo")).ok.body.json
        _ = try await client.getLibraryTimeline().ok.body.json
    }

    @Test func search() async throws {
        _ = try await client.search(query: .init(q: "al")).ok.body.json
    }

    @Test func anOperationOutsideTheDemoAnswersNotFound() async throws {
        let transport = DemoTransport()
        let (response, _) = try await transport.send(
            .init(method: .get, scheme: nil, authority: nil, path: "/nope"), body: nil,
            baseURL: URL(string: "https://demo.invalid")!, operationID: "nope")
        #expect(response.status == .notFound)
    }
}
