import Foundation
import SpotussiAPI

/// What History narrows its log to, as the web's History filters: one
/// entity, skipped, shuffle, counted, and one device or platform.
public struct HistoryFilter: Hashable, Sendable {
    /// A device seen by live capture, or a platform from the export.
    public enum Device: Hashable, Sendable {
        case device(String)
        case platform(String)

        public var name: String {
            switch self {
            case .device(let name), .platform(let name): name
            }
        }
    }

    /// The artist, album, track or genre the log is about. Set by the page
    /// that opens the log, so it isn't one of the filters a person sets.
    public var scope: EntityLink?
    public var skipped: Bool?
    public var shuffle: Bool?
    public var counted: Bool?
    public var device: Device?

    public init(
        scope: EntityLink? = nil, skipped: Bool? = nil, shuffle: Bool? = nil, counted: Bool? = nil,
        device: Device? = nil
    ) {
        self.scope = scope
        self.skipped = skipped
        self.shuffle = shuffle
        self.counted = counted
        self.device = device
    }

    /// The filters a person set; the scope is the page's own.
    public var activeCount: Int {
        [skipped != nil, shuffle != nil, counted != nil, device != nil].filter(\.self).count
    }

    /// The same log with every filter a person set taken off.
    public var cleared: HistoryFilter { HistoryFilter(scope: scope) }

    /// Sets the API's parameters for these filters on `query`.
    public func apply(to query: inout Operations.GetHistory.Input.Query) {
        query.artistId = scope?.kind == .artist ? scope?.id : nil
        query.albumId = scope?.kind == .album ? scope?.id : nil
        query.trackId = scope?.kind == .track ? scope?.id : nil
        query.genreId = scope?.kind == .genre ? scope?.id : nil
        query.skipped = skipped
        query.shuffle = shuffle
        query.counted = counted
        switch device {
        case .device(let name):
            query.device = name
            query.platform = nil
        case .platform(let name):
            query.device = nil
            query.platform = name
        case nil:
            query.device = nil
            query.platform = nil
        }
    }
}

/// Every row of the play log in a range, newest first, grouped by day as
/// the web's History groups it, paged by cursor until the server has no
/// more.
public struct HistoryLog: Hashable, Sendable {
    /// The plays of one local day.
    public struct Day: Identifiable, Hashable, Sendable {
        /// The day, `2026-10-21`.
        public let id: String
        /// Midnight starting it, in the calendar it was grouped in.
        public let date: Date
        public fileprivate(set) var plays: [RecentPlay]
    }

    public private(set) var days: [Day] = []
    /// Rows matching the filters, in the whole range.
    public private(set) var total: Int
    public private(set) var count = 0
    public private(set) var nextCursor: String?
    public private(set) var isComplete = false
    private var seen: Set<String> = []
    private let calendar: Calendar

    /// Rows a page asks for; the API allows up to 200.
    public static let pageSize = 100

    /// Starts the log with its first page, grouping days in `calendar`'s
    /// time zone.
    public init(first page: Components.Schemas.History, calendar: Calendar = .current) {
        self.calendar = calendar
        total = page.total
        append(page)
    }

    public var isEmpty: Bool { count == 0 }

    /// The play `fromEnd` rows before the last, for asking for the next
    /// page before the end is reached.
    public func play(fromEnd: Int) -> RecentPlay? {
        var remaining = fromEnd
        for day in days.reversed() {
            if remaining < day.plays.count { return day.plays[day.plays.count - 1 - remaining] }
            remaining -= day.plays.count
        }
        return days.first?.plays.first
    }

    /// Adds the next page. Rows already listed are dropped; a page with
    /// nothing new ends the log, as a server that ignores the cursor (the
    /// demo data) sends the first page again.
    public mutating func append(_ page: Components.Schemas.History) {
        total = page.total
        var fresh = 0
        for item in page.items where seen.insert(item.id).inserted {
            fresh += 1
            let play = RecentPlay(item)
            let start = calendar.startOfDay(for: play.playedAt)
            if let last = days.last, last.date == start {
                days[days.count - 1].plays.append(play)
            } else {
                let parts = calendar.dateComponents([.year, .month, .day], from: start)
                let key = String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
                days.append(Day(id: key, date: start, plays: [play]))
            }
        }
        count += fresh
        nextCursor = page.nextCursor
        isComplete = page.nextCursor == nil || fresh == 0
    }

    /// One page of the log for `range` and `filter`.
    public static func load(
        range: StatsRange, filter: HistoryFilter, cursor: String? = nil, limit: Int = pageSize, client: Client
    ) async throws -> Components.Schemas.History {
        let span = range.query
        var query = Operations.GetHistory.Input.Query(
            range: span.range, from: span.from, to: span.to, cursor: cursor, limit: limit)
        filter.apply(to: &query)
        return try await client.getHistory(query: query).ok.body.json
    }
}

/// The devices and platforms the log can be narrowed to, busiest first.
public struct PlaybackDevices: Hashable, Sendable {
    /// Named by live capture: "iPhone", "Kitchen".
    public let devices: [String]
    /// From the export: "ios", "android".
    public let platforms: [String]

    public init(_ devices: Components.Schemas.Devices) {
        self.devices = devices.devices.map(\.name)
        platforms = devices.platforms.map(\.name)
    }

    public var isEmpty: Bool { devices.isEmpty && platforms.isEmpty }
}
