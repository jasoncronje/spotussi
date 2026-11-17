import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

@Suite struct LibraryTests {
    let us = Locale(identifier: "en_US")
    let client = Client.demo

    var utc: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }

    // MARK: - Paging

    @Test func demoSongsStopAfterTheOnlyPage() async throws {
        let query = SavedSong.Query(filter: .neverPlayed, sort: .added)
        var list = OffsetList(first: try await SavedSong.page(query, offset: 0, limit: 50, client: client))
        #expect(list.items.count == 25)
        #expect(list.total == 1_454)
        #expect(!list.isComplete)

        // The demo answers every offset with the first page: the list must
        // end rather than repeat it.
        list.append(
            try await SavedSong.page(query, offset: list.nextOffset, limit: 50, client: client),
            requestedAt: list.nextOffset)
        #expect(list.isComplete)
        #expect(list.items.count == 25)
    }

    @Test func demoAlbumsAndPlaylistsStopToo() async throws {
        var albums = OffsetList(first: try await SavedAlbum.page(.added, offset: 0, limit: 50, client: client))
        albums.append(
            try await SavedAlbum.page(.added, offset: albums.nextOffset, limit: 50, client: client),
            requestedAt: albums.nextOffset)
        #expect(albums.isComplete)
        #expect(albums.items.count == 25)

        let query = SavedPlaylist.Query(relation: .all, sort: .playsHere)
        var playlists = OffsetList(first: try await SavedPlaylist.page(query, offset: 0, limit: 50, client: client))
        playlists.append(
            try await SavedPlaylist.page(query, offset: playlists.nextOffset, limit: 50, client: client),
            requestedAt: playlists.nextOffset)
        #expect(playlists.isComplete)
        #expect(playlists.total == 48)
    }

    @Test func aPlaylistThatFitsOnOnePageIsComplete() async throws {
        let detail = try await SavedPlaylist.detail(id: "demo", limit: 100, client: client)
        let songs = OffsetList(first: detail.songs)
        #expect(songs.items.count == 45)
        #expect(songs.isComplete)
        #expect(detail.playlist.name == "Running Winter")
        #expect(detail.playlist.relation == .collaborative)
        #expect(detail.playlist.playThrough == PlayThrough(played: 43, total: 45))
        // Collaborative: each song says who added it.
        #expect(songs.items.first?.adder == .you)
        #expect(songs.items.first?.position == 0)
        #expect(songs.items.first?.playsHere == 3)
    }

    @Test func aPlaylistNoLongerInTheLibrary() async throws {
        let transport = StubTransport { _ in Reply.apiError(404, code: "not_found") }
        await #expect(throws: EntityLoadError.notFound) {
            _ = try await SavedPlaylist.detail(id: "gone", limit: 100, client: transport.apiClient)
        }
    }

    @Test func pagesOfTheSameSongSavedTwiceKeepBoth() {
        // A song removed and saved again is a new save with its own date.
        let first = song(id: "t1", added: 1_000)
        let again = song(id: "t1", added: 2_000)
        let list = OffsetList(first: OffsetPage(items: [first, again, first], offset: 0, total: 3))
        #expect(list.items.count == 2)
    }

    // MARK: - Song rows

    @Test func rotationLampsStopAtHeavyRotation() {
        #expect(song(plays: 0).litLamps() == 0)
        #expect(song(plays: 3).litLamps() == 3)
        #expect(song(plays: 17).litLamps() == 5)
    }

    @Test func theNoteUnderASong() {
        let day = 86_400_000
        #expect(song(status: .neverPlayed).note() == .neverPlayed)
        #expect(song(status: .neverPlayed).note(quiet: true) == nil)
        #expect(song(status: .maybePlayed).note() == .otherRelease)
        #expect(song(added: 10 * day, rotation: 9 * day).note() == .rotation(.beforeSaving))
        #expect(song(added: 10 * day, rotation: 10 * day + 3_600_000).note() == .rotation(.sameDay))
        #expect(song(added: 10 * day, rotation: 13 * day).note() == .rotation(.days(3)))
        #expect(song(added: nil, rotation: 13 * day).note() == .rotation(nil))
        #expect(song(lastPlayed: 5 * day).note() == .lastPlayed(Date(milliseconds: 5 * day)))
        #expect(song().note() == nil)
    }

    @Test func demoSongsDecode() async throws {
        let page = try await SavedSong.page(
            SavedSong.Query(filter: .all, sort: .added), offset: 0, limit: 50, client: client)
        let first = try #require(page.items.first)
        #expect(first.track.name == "The Tangled Signal")
        #expect(first.artists == "Ezra Quiveto")
        #expect(first.plays == 17)
        #expect(first.note() == .rotation(.beforeSaving))
        #expect(first.adder == nil)
    }

    // MARK: - Play-through

    @Test func playThroughDots() throws {
        let few = try #require(PlayThrough(played: 3, total: 7))
        #expect(few.dots().count == 7)
        #expect(few.dots().lit == 3)
        let many = try #require(PlayThrough(played: 43, total: 45))
        #expect(many.dots(limit: 14).count == 14)
        #expect(abs(many.dots(limit: 14).lit - 43.0 / 45 * 14) < 0.0001)
        #expect(PlayThrough(played: 0, total: 0) == nil)
        #expect(PlayThrough(played: 1, total: nil) == nil)
    }

    // MARK: - Condition

    @Test func savesShowWhateverTheSyncSays() throws {
        let condition = LibraryCondition(summary: try summary(), status: try status())
        #expect(condition.empty == nil)
        #expect(!condition.needsScope)
        #expect(condition.syncing == nil)
    }

    @Test func whyAnEmptyLibraryIsEmpty() throws {
        let empty = try summary(empty: true)
        #expect(LibraryCondition(summary: empty, status: try status { $0.state = .disabled }).empty == .syncOff)
        #expect(
            LibraryCondition(summary: empty, status: try status { $0.state = .notConnected }).empty == .notConnected)
        #expect(
            LibraryCondition(summary: empty, status: try status { $0.lists.tracks.state = .needsScope }).empty
                == .needsScope)
        #expect(LibraryCondition(summary: empty, status: try status { $0.lastSyncAt = nil }).empty == .firstSync)
        #expect(LibraryCondition(summary: empty, status: try status()).empty == .nothing)
        #expect(LibraryCondition(summary: empty, status: nil).empty == .nothing)
    }

    @Test func playlistsWithoutTheLibraryPermission() throws {
        let condition = LibraryCondition(
            summary: try summary(), status: try status { $0.lists.albums.state = .needsScope })
        #expect(condition.empty == nil)
        #expect(condition.needsScope)
    }

    @Test func aFirstSyncPartWay() throws {
        let condition = LibraryCondition(
            summary: try summary(empty: true),
            status: try status {
                $0.state = .syncing
                $0.lastSyncAt = nil
                $0.progress = 0.38
            })
        #expect(condition.empty == .firstSync)
        #expect(condition.syncing?.fraction == 0.38)
        #expect(condition.syncing?.litLamps(of: 24) == 9)

        let unknown = LibraryCondition(
            summary: try summary(empty: true),
            status: try status {
                $0.state = .syncing
                $0.progress = nil
            })
        #expect(unknown.syncing == LibraryCondition.Syncing(fraction: nil))
    }

    // MARK: - Rotation mix

    @Test func mixRunsFillTheRow() throws {
        let mix = RotationMix(try summary().rotationMix)
        let runs = mix.runs()
        #expect(runs.reduce(0, +) == 30)
        // Every part with songs gets a dot, however small.
        #expect(zip(mix.counts, runs).allSatisfy { count, run in count == 0 ? run == 0 : run >= 1 })
        #expect(mix.total == 1_454)
        #expect(mix.shown == RotationMix.Part.allCases)
    }

    @Test func mixRunsByLargestRemainder() {
        let mix = RotationMix(counts: [1, 1, 1, 0, 0, 0])
        #expect(mix.runs(dots: 10) == [4, 3, 3, 0, 0, 0])
        #expect(RotationMix(counts: [0, 0, 0, 0, 0, 0]).runs() == [0, 0, 0, 0, 0, 0])
        #expect(RotationMix(counts: [0, 0, 0, 0, 0, 5]).shown == [.notYet])
    }

    @Test func mixPercentages() {
        #expect(RotationMix.percent(0.46, locale: us) == "46%")
        #expect(RotationMix.percent(0.004, locale: us) == "<1%")
        #expect(RotationMix.percent(0, locale: us) == "0%")
    }

    // MARK: - Saves over time

    @Test func savesGroupToFitTheWidth() async throws {
        let timeline = SavesTimeline(try await client.getLibraryTimeline(query: .init(range: .all, bucket: .month)).ok.body.json)
        #expect(timeline.columns(months: 1).count == 61)
        #expect(timeline.saved == timeline.columns(months: 12).reduce(0) { $0 + $1.saved })
        // 61 months in 330 points are under 7 points each: quarters.
        let quarters = timeline.columns(fitting: 330)
        #expect(quarters.first?.months == 3)
        #expect(quarters.first?.year == 2021)
        #expect(quarters.first?.month == 10)
        #expect(timeline.columns(fitting: 1000).first?.months == 1)
        #expect(timeline.columns(fitting: 40).first?.months == 12)
    }

    @Test func savesLamps() {
        let busiest = SavesTimeline.Column(id: "a", year: 2024, month: 1, months: 1, saved: 90, removed: 0)
        #expect(SavesTimeline.lamps(busiest, peak: 90) == Array(repeating: 1, count: 9))
        let half = SavesTimeline.Column(id: "b", year: 2024, month: 2, months: 1, saved: 45, removed: 2)
        let lamps = SavesTimeline.lamps(half, peak: 90)
        #expect(lamps.prefix(4) == [1, 1, 1, 1])
        #expect(lamps[4] == 0.5)
        #expect(lamps[5] == 0)
        #expect(SavesTimeline.removalLamps(half, peak: 10) == 1)
        #expect(SavesTimeline.removalLamps(half, peak: 2) == 3)
        let none = SavesTimeline.Column(id: "c", year: 2024, month: 3, months: 1, saved: 0, removed: 0)
        #expect(SavesTimeline.lamps(none, peak: 90).allSatisfy { $0 == 0 })
        #expect(SavesTimeline.removalLamps(none, peak: 2) == 0)
        // A sliver still shows, dimly.
        let one = SavesTimeline.Column(id: "d", year: 2024, month: 4, months: 1, saved: 1, removed: 0)
        #expect(SavesTimeline.lamps(one, peak: 90)[0] == 0.35)
    }

    // MARK: - Figures

    @Test func medianDays() {
        #expect(Figures.days(nil, locale: us) == "–")
        #expect(Figures.days(2.5, locale: us) == "2.5")
        #expect(Figures.days(3, locale: us) == "3")
        #expect(Figures.days(50.1, locale: us) == "50")
        #expect(Figures.days(1_266.2, locale: us) == "1,266")
    }

    @Test func savedDates() throws {
        let now = try #require(utc.date(from: DateComponents(year: 2026, month: 11, day: 17)))
        let thisYear = try #require(utc.date(from: DateComponents(year: 2026, month: 10, day: 14)))
        let before = try #require(utc.date(from: DateComponents(year: 2024, month: 3, day: 2)))
        #expect(Figures.date(thisYear, now: now, calendar: utc, locale: us) == "Oct 14")
        #expect(Figures.date(before, now: now, calendar: utc, locale: us) == "Mar 2, 2024")
    }

    // MARK: - Helpers

    private func song(
        id: String = "t", added: Int? = nil, status: SavedSong.Status = .played, plays: Int = 1,
        lastPlayed: Int? = nil, rotation: Int? = nil
    ) -> SavedSong {
        SavedSong(
            Components.Schemas.LibraryTrack(
                addedAt: added.map(Int64.init), lastPlayedAt: lastPlayed.map(Int64.init), msPlayed: 0, plays: plays,
                rotationAt: rotation.map(Int64.init), status: status,
                track: .init(artists: [], id: id, name: "Song")))
    }

    private func summary(empty: Bool = false) throws -> Components.Schemas.LibrarySummary {
        var summary = try JSONDecoder().decode(
            Components.Schemas.LibrarySummary.self, from: try #require(Fixtures.data(for: "getLibrarySummary")))
        if empty {
            summary.savedTracks = 0
            summary.savedAlbums = 0
            summary.playlists = 0
        }
        return summary
    }

    private func status(
        _ change: (inout Components.Schemas.LibraryStatus) -> Void = { _ in }
    ) throws -> Components.Schemas.LibraryStatus {
        var library = try fixtureStatus().library
        change(&library)
        return library
    }
}
