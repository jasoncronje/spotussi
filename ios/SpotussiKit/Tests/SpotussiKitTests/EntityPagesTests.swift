import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

/// Top lists paged without a cap, the four entity pages, play contexts and
/// search, reduced from the demo data the way the web's views read them.
@Suite struct RankedListTests {
    @Test func aServerRepeatingThePageEndsTheList() async throws {
        let client = fixtureClient()
        let first = try await TopPage.load(.track, range: .default, measure: .plays, limit: 50, client: client)
        var list = RankedList(first: first)
        #expect(list.entries.count == 25)
        #expect(list.total == 750)
        #expect(!list.isComplete)
        #expect(list.nextOffset == 25)

        // A server that answers every offset with the first page: the list
        // must end rather than repeat it.
        let again = try await TopPage.load(
            .track, range: .default, measure: .plays, offset: list.nextOffset, limit: 50, client: client)
        list.append(again, requestedAt: 25)
        #expect(list.isComplete)
        #expect(list.entries.count == 25)
    }

    @Test(arguments: EntityKind.allCases)
    func everyKindLoads(_ kind: EntityKind) async throws {
        let page = try await TopPage.load(kind, range: .default, measure: .time, limit: 50, client: .demo)
        #expect(!page.entries.isEmpty)
        #expect(page.entries.allSatisfy { $0.kind == kind })
        #expect(page.hasPrevious)
        #expect((page.entries.first?.spotifyURI != nil) == (kind != .genre))
    }

    @Test func pagesUntilTheTotal() {
        var list = RankedList(first: page(ranks: Array(1...2), offset: 0, total: 5))
        #expect(!list.isComplete)
        list.append(page(ranks: Array(3...4), offset: 2, total: 5), requestedAt: 2)
        #expect(!list.isComplete)
        list.append(page(ranks: Array(5...5), offset: 4, total: 5), requestedAt: 4)
        #expect(list.isComplete)
        #expect(list.entries.map(\.rank) == [1, 2, 3, 4, 5])
    }

    @Test func anEmptyRankingIsComplete() {
        let list = RankedList(first: page(ranks: [], offset: 0, total: 0))
        #expect(list.isComplete)
        #expect(list.entries.isEmpty)
        #expect(list.leader(for: .plays) == 1)
    }

    @Test func anEmptyPageEndsTheList() {
        var list = RankedList(first: page(ranks: Array(1...2), offset: 0, total: 9))
        list.append(page(ranks: [], offset: 2, total: 9), requestedAt: 2)
        #expect(list.isComplete)
    }

    @Test func entriesListedAlreadyAreDropped() {
        var list = RankedList(first: page(ranks: Array(1...3), offset: 0, total: 6))
        list.append(page(ranks: Array(3...5), offset: 3, total: 6), requestedAt: 3)
        #expect(list.entries.map(\.rank) == [1, 2, 3, 4, 5])
        #expect(list.nextOffset == 6)
        #expect(list.isComplete)
    }

    @Test func theLeaderIsTheFirstEntry() {
        let list = RankedList(first: page(ranks: Array(1...3), offset: 0, total: 3))
        #expect(list.leader(for: .plays) == 100)
        #expect(list.leader(for: .time) == 100_000)
    }

    /// Entries ranked by plays, falling by ten a rank from 100.
    private func page(ranks: [Int], offset: Int, total: Int) -> TopPage {
        let entries = ranks.map { rank in
            TopEntry(
                kind: .track, entityID: "t\(rank)", rank: rank, name: "Track \(rank)", detail: nil,
                imageURL: nil, plays: 110 - rank * 10, milliseconds: (110 - rank * 10) * 1000, movement: nil)
        }
        return TopPage(entries: entries, offset: offset, total: total, hasPrevious: false)
    }
}

@Suite struct EntityDetailTests {
    @Test func artist() async throws {
        let artist = try await EntityDetail.load(
            .artist, id: "anything", range: .default, measure: .plays, client: .demo)
        #expect(artist.link == EntityLink(kind: .artist, id: "rWPmqZoeZKBX2dWqnMTX5p", name: "Iris Sakoti"))
        #expect(artist.kicker == .artist)
        #expect(artist.spotifyURI == "spotify:artist:rWPmqZoeZKBX2dWqnMTX5p")
        #expect(artist.genres.map(\.name) == ["dutch house", "modern house"])
        #expect(artist.genres.allSatisfy { $0.kind == .genre })
        #expect(artist.rank == 1)
        #expect(artist.rankedTotal == 568)
        #expect(artist.inRange == .init(plays: 21, milliseconds: 4_051_180, skips: 8))
        #expect(artist.allTime.plays == 168)
        #expect(artist.firstPlayed == Date(timeIntervalSince1970: 1_726_610_026.251))
        #expect(artist.lastPlayed == Date(timeIntervalSince1970: 1_794_422_939.381))
        #expect(!artist.isExcluded)
        #expect(artist.isListened)
    }

    @Test func album() async throws {
        let album = try await EntityDetail.load(.album, id: "x", range: .default, measure: .plays, client: .demo)
        #expect(album.name == "The Honest Paradise")
        #expect(album.kicker == .single)
        #expect(album.releaseDate == "2014-12-11")
        #expect(album.trackCount == 1)
        #expect(album.artists.map(\.name) == ["Iris Sakoti"])
        #expect(album.spotifyURI == "spotify:album:XU7VKyfHJ1vtfOQKccB6GZ")
    }

    @Test func track() async throws {
        let track = try await EntityDetail.load(.track, id: "x", range: .default, measure: .plays, client: .demo)
        #expect(track.kicker == .track)
        #expect(track.album == EntityLink(kind: .album, id: "XU7VKyfHJ1vtfOQKccB6GZ", name: "The Honest Paradise"))
        #expect(track.artists == [EntityLink(kind: .artist, id: "rWPmqZoeZKBX2dWqnMTX5p", name: "Iris Sakoti")])
        #expect(track.duration == 194_083)
        #expect(track.isExplicit)
        #expect(track.savedAt == nil)
        #expect(track.spotifyURI == "spotify:track:wbS3CeSCggo6czKa8yQRXQ")
    }

    @Test func genre() async throws {
        let genre = try await EntityDetail.load(.genre, id: "x", range: .default, measure: .plays, client: .demo)
        #expect(genre.link == EntityLink(kind: .genre, id: "modern-pop", name: "modern pop"))
        #expect(genre.artistCount == 37)
        #expect(genre.imageURL == nil)
        #expect(genre.spotifyURI == nil)
        #expect(genre.inRange.value(for: .time) == 16_289_903)
    }

    @Test func aMissingEntityIsNotFound() async throws {
        let transport = StubTransport { _ in Reply.apiError(404, code: "not_found") }
        await #expect(throws: EntityLoadError.notFound) {
            try await EntityDetail.load(
                .artist, id: "gone", range: .default, measure: .plays, client: transport.apiClient)
        }
    }

    @Test(arguments: [
        ("2014-12-11", "Dec 11, 2014"),
        ("2014-12", "Dec 2014"),
        ("2014", "2014"),
    ])
    func releaseDate(_ text: String, _ expected: String) {
        #expect(Figures.releaseDate(text, locale: Locale(identifier: "en_US")) == expected)
    }

    @Test(arguments: ["", "2014-13", "14", "2014-12-32", "soon"])
    func releaseDateNonsense(_ text: String) {
        #expect(Figures.releaseDate(text, locale: Locale(identifier: "en_US")) == nil)
    }
}

@Suite struct PlayContextTests {
    @Test func contextsFromTheDemo() async throws {
        let contexts = PlayContexts(try await Client.demo.getContexts().ok.body.json)
        let first = try #require(contexts.items.first)
        #expect(first.kind == .likedSongs)
        #expect(first.name == "Liked Songs")
        #expect(first.rank == 1)
        #expect(contexts.leader(for: .plays) == first.plays)
        let named = try #require(contexts.items.first { $0.name == "Daily Mix 1" })
        #expect(named.isSpotifyPlaylist)
        #expect(named.isNamedByOwner)
        #expect(!contexts.isEmpty(for: .plays))
    }
}

@Suite struct SearchTests {
    @Test(arguments: [
        ("", nil),
        (" a ", nil),
        ("ab", "ab"),
        ("  iris  ", "iris"),
    ] as [(String, String?)])
    func query(_ text: String, _ expected: String?) {
        #expect(CatalogSearch.query(text) == expected)
    }

    @Test func groupsInTheAPIsOrder() async throws {
        let results = try await Client.demo.search(query: .init(q: "iris")).ok.body.json
        let groups = SearchGroup.list(results)
        #expect(groups.map(\.kind) == [.artist, .album, .track])
        #expect(groups.map(\.hits.count) == [5, 2, 1])
        let album = try #require(groups.first { $0.kind == .album }?.hits.first)
        #expect(album.link.name == "Ririel")
        #expect(album.detail == "Horizon Postcard")
        #expect(Set(groups.flatMap(\.hits).map(\.id)).count == groups.flatMap(\.hits).count)
    }

    @Test func noResultsMeansNoGroups() {
        #expect(SearchGroup.list(.init(albums: [], artists: [], tracks: [])).isEmpty)
    }
}

@Suite struct SpotifyLinkTests {
    @Test(arguments: [
        ("spotify:track:11dFghVXANMlKmJXsNCbNl", "https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl"),
        ("spotify:artist:0OdUWJ0sBjDrqHygGUXeCF", "https://open.spotify.com/artist/0OdUWJ0sBjDrqHygGUXeCF"),
        ("spotify:album:4aawyAB9vmqN3uQ7FjRGTy", "https://open.spotify.com/album/4aawyAB9vmqN3uQ7FjRGTy"),
    ])
    func webAddress(_ uri: String, _ expected: String) {
        #expect(SpotifyLink.url(for: uri)?.absoluteString == expected)
    }

    @Test(arguments: ["", "spotify:track:", "spotify:user:me:collection", "https://example.com", "spotify:track:a/b"])
    func notAURI(_ uri: String) {
        #expect(SpotifyLink.url(for: uri) == nil)
    }
}
