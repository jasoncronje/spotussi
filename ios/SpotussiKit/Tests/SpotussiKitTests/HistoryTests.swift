import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

/// The play log paged by cursor and grouped by day, its filters, and the
/// longest sessions, as the web's History and Sessions read them.
@Suite struct HistoryTests {
    let us = Locale(identifier: "en_US")
    let client = Client.demo

    var utc: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }

    // MARK: - The log

    @Test func demoPagingStopsAfterTheOnlyPage() async throws {
        let first = try await HistoryLog.load(range: .default, filter: HistoryFilter(), client: client)
        var log = HistoryLog(first: first, calendar: utc)
        #expect(log.count == 25)
        #expect(log.total == 49_981)
        #expect(log.nextCursor == "12jo")
        #expect(!log.isComplete)

        // The demo answers every cursor with the first page: the log must
        // end rather than repeat it.
        log.append(try await HistoryLog.load(range: .default, filter: HistoryFilter(), cursor: "12jo", client: client))
        #expect(log.isComplete)
        #expect(log.count == 25)
    }

    @Test func playsAreGroupedByDayNewestFirst() throws {
        let log = HistoryLog(first: try page(ids: ["a", "b", "c"], hoursAgo: [1, 25, 26], cursor: nil), calendar: utc)
        #expect(log.days.count == 2)
        #expect(log.days[0].plays.map(\.id) == ["a"])
        #expect(log.days[1].plays.map(\.id) == ["b", "c"])
        #expect(log.days[0].date > log.days[1].date)
        #expect(log.isComplete)
    }

    @Test func aNextPageContinuesTheLastDay() throws {
        var log = HistoryLog(first: try page(ids: ["a", "b"], hoursAgo: [1, 25], cursor: "next"), calendar: utc)
        #expect(!log.isComplete)
        log.append(try page(ids: ["b", "c", "d"], hoursAgo: [25, 25.5, 50], cursor: "more"))
        #expect(log.days.map { $0.plays.map(\.id) } == [["a"], ["b", "c"], ["d"]])
        #expect(log.count == 4)
        #expect(!log.isComplete)
        #expect(log.play(fromEnd: 0)?.id == "d")
        #expect(log.play(fromEnd: 2)?.id == "b")
        #expect(log.play(fromEnd: 10)?.id == "a")
    }

    @Test func anEmptyLogIsComplete() throws {
        let log = HistoryLog(first: try page(ids: [], hoursAgo: [], cursor: nil), calendar: utc)
        #expect(log.isEmpty)
        #expect(log.isComplete)
        #expect(log.days.isEmpty)
    }

    @Test func playsCarryWhatTheRowSays() async throws {
        let first = try await HistoryLog.load(range: .default, filter: HistoryFilter(), client: client)
        let play = RecentPlay(try #require(first.items.first))
        #expect(play.isCounted)
        #expect(play.endedAt > play.playedAt)
    }

    // MARK: - Filters

    @Test func filtersBecomeQueryParameters() {
        let filter = HistoryFilter(
            scope: EntityLink(kind: .album, id: "al1", name: "Album"), skipped: true, shuffle: false,
            device: .platform("ios"))
        var query = Operations.GetHistory.Input.Query(device: "stale")
        filter.apply(to: &query)
        #expect(query.albumId == "al1")
        #expect(query.artistId == nil && query.trackId == nil && query.genreId == nil)
        #expect(query.skipped == true)
        #expect(query.shuffle == false)
        #expect(query.counted == nil)
        #expect(query.device == nil)
        #expect(query.platform == "ios")
    }

    @Test func theScopeIsntCountedOrCleared() {
        let scope = EntityLink(kind: .artist, id: "ar1", name: "Iris Sakoti")
        let filter = HistoryFilter(scope: scope, counted: false, device: .device("Kitchen"))
        #expect(filter.activeCount == 2)
        #expect(filter.cleared == HistoryFilter(scope: scope))
        #expect(filter.cleared.activeCount == 0)
        #expect(HistoryFilter.Device.device("Kitchen").name == "Kitchen")
    }

    @Test func devicesFromTheDemo() async throws {
        let devices = PlaybackDevices(try await client.getDevices().ok.body.json)
        #expect(devices.devices.first == "iPhone")
        #expect(devices.platforms.first == "ios")
        #expect(!devices.isEmpty)
    }

    @Test func dayTitles() throws {
        let now = try #require(utc.date(from: DateComponents(year: 2026, month: 11, day: 17, hour: 9)))
        let hour = 3_600.0
        #expect(Figures.day(now.addingTimeInterval(-hour), now: now, calendar: utc, locale: us) == "Today")
        #expect(Figures.day(now.addingTimeInterval(-20 * hour), now: now, calendar: utc, locale: us) == "Yesterday")
        #expect(Figures.day(now.addingTimeInterval(-60 * hour), now: now, calendar: utc, locale: us) == "Sat, Nov 14")
        #expect(
            Figures.day(now.addingTimeInterval(-400 * 24 * hour), now: now, calendar: utc, locale: us)
                == "Mon, Oct 13, 2025")
    }

    // MARK: - Sessions

    @Test func demoSessionsStopAfterTheOnlyPage() async throws {
        var list = SessionList(first: try await SessionList.load(range: .default, client: client))
        #expect(list.sessions.count == 25)
        #expect(list.total == 102)
        #expect(list.gapMinutes == 15)
        #expect(!list.isComplete)
        let first = try #require(list.sessions.first)
        #expect(list.longest == first.duration)
        #expect(first.topArtists.count == 3)

        list.append(
            try await SessionList.load(range: .default, offset: list.nextOffset, client: client),
            requestedAt: list.nextOffset)
        #expect(list.isComplete)
        #expect(list.sessions.count == 25)
    }

    @Test func aSessionAndItsTape() async throws {
        let (session, plays) = try await ListeningSession.load(id: "49024", client: client)
        #expect(session.rows == plays.count)
        #expect(session.plays == 32)
        #expect(session.skips == 9)
        #expect(session.silence == 6_003_777 - 5_964_064)
        #expect(session.artistNames == "Bitter Parade, DJ Elny, The Midnight Lanterns")
        let tape = SessionTape(session: session, plays: plays)
        #expect(tape.marks.count == plays.count)
        #expect(tape.marks.first?.start == 0)
        #expect(tape.marks.allSatisfy { (0...1).contains($0.start) && $0.end >= $0.start && $0.end <= 1 })
        #expect(tape.marks.filter(\.isLit).count == plays.filter { $0.isCounted && !$0.isSkipped }.count)
    }

    @Test func aSessionThatIsGone() async throws {
        let transport = StubTransport { _ in Reply.apiError(404, code: "not_found") }
        await #expect(throws: EntityLoadError.notFound) {
            _ = try await ListeningSession.load(id: "gone", client: transport.apiClient)
        }
    }

    // MARK: - Helpers

    /// A page of the log: the demo's first row under new ids, played the
    /// given hours before 2026-11-17 09:00 UTC.
    func page(ids: [String], hoursAgo: [Double], cursor: String?) throws -> Components.Schemas.History {
        var history = try JSONDecoder().decode(
            Components.Schemas.History.self, from: Fixtures.data(for: "getHistory")!)
        let template = history.items[0]
        let now: Int64 = 1_794_906_000_000
        history.items = zip(ids, hoursAgo).map { id, hours in
            var play = template
            play.id = id
            play.playedAt = now - Int64(hours * 3_600_000)
            play.endedAt = play.playedAt + 200_000
            return play
        }
        history.nextCursor = cursor
        history.total = ids.count
        return history
    }
}
