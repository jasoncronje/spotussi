import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

/// Top lists and recent plays, reduced from the demo data the way the web's
/// Overview adapters reduce them.
@Suite struct OverviewDataTests {
    let client = Client.demo

    @Test func topTracksKeepTheFirstFive() async throws {
        let entries = TopEntry.list(try await client.getTopTracks().ok.body.json)
        #expect(entries.map(\.rank) == [1, 2, 3, 4, 5])
        let first = try #require(entries.first)
        #expect(first.kind == .track)
        #expect(first.name == "The Honest Paradise")
        #expect(first.detail == "Iris Sakoti")
        #expect(first.imageURL == "/_mock/img/album/XU7VKyfHJ1vtfOQKccB6GZ.svg")
        #expect(first.plays == 18)
        #expect(first.value(for: .time) == 3_286_777)
        #expect(first.movement == .up(1, from: 2))
        #expect(first.figure(for: .plays, locale: Locale(identifier: "en_US")) == "18")
        #expect(plain(first.figure(for: .time, locale: Locale(identifier: "en_US"))) == "54m")
    }

    @Test func everyKindHasItsDetails() async throws {
        let artists = TopEntry.list(try await client.getTopArtists().ok.body.json)
        #expect(artists.first?.detail == nil)
        #expect(artists.first?.imageURL?.hasPrefix("/_mock/img/artist/") == true)
        #expect(artists.first?.movement == .up(12, from: 13))

        let albums = TopEntry.list(try await client.getTopAlbums().ok.body.json)
        #expect(albums.first?.detail == "Iris Sakoti")

        let genres = TopEntry.list(try await client.getTopGenres().ok.body.json)
        #expect(genres.first?.name == "modern pop")
        #expect(genres.allSatisfy { $0.imageURL == nil && $0.kind == .genre })
        #expect(Set(genres.map(\.id)).count == genres.count)
    }

    @Test(arguments: [
        (3, 5, Movement.up(2, from: 5)),
        (5, 3, Movement.down(2, from: 3)),
        (4, 4, Movement.same),
    ])
    func movement(_ rank: Int, _ previous: Int, _ expected: Movement) {
        #expect(Movement(rank: rank, previousRank: previous, hasPrevious: true) == expected)
    }

    @Test func movementNeedsAPreviousPeriod() {
        #expect(Movement(rank: 1, previousRank: nil, hasPrevious: true) == .new)
        #expect(Movement(rank: 1, previousRank: nil, hasPrevious: false) == nil)
        #expect(Movement(rank: 1, previousRank: 4, hasPrevious: false) == nil)
    }

    @Test func recentPlaysAreTheLatestTen() async throws {
        let history = try await client.getHistory().ok.body.json
        let plays = RecentPlay.list(history)
        #expect(plays.count == 10)
        let first = try #require(plays.first)
        #expect(first.name == "Sinking")
        #expect(first.artists == "Elin Gabe, The Distant Satellites")
        #expect(first.playedAt == Date(timeIntervalSince1970: 1_794_908_281.599))
        // Every note follows its play's flags, skipped first.
        for (play, item) in zip(plays, history.items) {
            switch play.note {
            case .skipped(let at): #expect(item.skipped && at == item.msPlayed)
            case .notCounted: #expect(!item.skipped && !item.countsAsPlay)
            case .estimated: #expect(!item.skipped && item.countsAsPlay && item.msEstimated)
            case nil: #expect(!item.skipped && item.countsAsPlay && !item.msEstimated)
            }
        }
    }
}
