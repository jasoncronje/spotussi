import Foundation
import SpotussiAPI

/// Listening over the 24 local hours, as the web's listening-clock.svelte
/// draws it: each hour a spoke of lamps lit to its share of the busiest
/// hour. From `/stats/hourly-top` each hour also knows its top artist and
/// track, as the web's "Who you play at each hour".
public struct HourlyListening: Hashable, Sendable {
    public struct Hour: Identifiable, Hashable, Sendable {
        /// 0 to 23.
        public let id: Int
        public let plays: Int
        public let milliseconds: Int
        public let topArtist: Leader?
        public let topTrack: Leader?

        public func value(for measure: Measure) -> Int {
            measure == .plays ? plays : milliseconds
        }
    }

    /// The artist or track that led an hour.
    public struct Leader: Hashable, Sendable {
        public let link: EntityLink
        /// A track's artists.
        public let detail: String?
        public let imageURL: String?
        public let plays: Int
        public let milliseconds: Int

        public func value(for measure: Measure) -> Int {
            measure == .plays ? plays : milliseconds
        }
    }

    /// Always 24, midnight first.
    public let hours: [Hour]
    public let measure: Measure

    /// Lamps on each spoke.
    public static let rings = 8

    public init(_ clock: Components.Schemas.Clock, measure: Measure) {
        let byHour = Dictionary(clock.hours.map { ($0.hour, $0) }, uniquingKeysWith: { first, _ in first })
        hours = (0..<24).map { hour in
            let item = byHour[hour]
            return Hour(
                id: hour, plays: item?.plays ?? 0, milliseconds: item?.msPlayed ?? 0, topArtist: nil,
                topTrack: nil)
        }
        self.measure = measure
    }

    public init(_ top: Components.Schemas.HourlyTop, measure: Measure) {
        let byHour = Dictionary(top.hours.map { ($0.hour, $0) }, uniquingKeysWith: { first, _ in first })
        hours = (0..<24).map { hour in
            guard let item = byHour[hour] else {
                return Hour(id: hour, plays: 0, milliseconds: 0, topArtist: nil, topTrack: nil)
            }
            let artist = item.topArtist.map { leader in
                Leader(
                    link: EntityLink(kind: .artist, id: leader.artist.id, name: leader.artist.name), detail: nil,
                    imageURL: leader.artist.imageUrl, plays: leader.plays, milliseconds: leader.msPlayed)
            }
            let track = item.topTrack.map { leader in
                Leader(
                    link: EntityLink(kind: .track, id: leader.track.id, name: leader.track.name),
                    detail: TopEntry.names(leader.track.artists), imageURL: leader.track.imageUrl,
                    plays: leader.plays, milliseconds: leader.msPlayed)
            }
            return Hour(id: hour, plays: item.plays, milliseconds: item.msPlayed, topArtist: artist, topTrack: track)
        }
        self.measure = measure
    }

    public func value(_ hour: Hour) -> Int { hour.value(for: measure) }

    public var total: Int { hours.reduce(0) { $0 + value($1) } }

    public var isEmpty: Bool { total == 0 }

    /// The busiest hour, the earliest on a tie; nil when nothing was played.
    public var peak: Hour? {
        guard let top = hours.map(value).max(), top > 0 else { return nil }
        return hours.first { value($0) == top }
    }

    /// The quietest hour, the earliest on a tie; nil when nothing was played.
    public var quietest: Hour? {
        guard !isEmpty, let low = hours.map(value).min() else { return nil }
        return hours.first { value($0) == low }
    }

    /// The hour's part of the whole day, 0...1.
    public func share(_ hour: Hour) -> Double {
        let total = total
        return total > 0 ? Double(value(hour)) / Double(total) : 0
    }

    /// How brightly lamp `ring` (0 innermost) on the hour's spoke is lit,
    /// 0...1. The outermost lit lamp is lit in part, so nothing is rounded
    /// away, and any listening lights a little of the first lamp.
    public func light(_ hour: Hour, ring: Int, rings: Int = rings) -> Double {
        let value = value(hour)
        let busiest = hours.map(self.value).max() ?? 0
        guard value > 0, busiest > 0 else { return 0 }
        let level = max(Double(value) / Double(busiest) * Double(rings), 0.35)
        return min(1, max(0, level - Double(ring)))
    }

    /// The artist leading the most hours and how many, as the web's summary
    /// of the hourly table; nil when no hour has a leader.
    public var leadingArtist: (link: EntityLink, hours: Int)? {
        var counts: [String: (link: EntityLink, hours: Int, first: Int)] = [:]
        for hour in hours {
            guard let artist = hour.topArtist else { continue }
            counts[artist.link.id, default: (artist.link, 0, hour.id)].hours += 1
        }
        let best = counts.values.max { lhs, rhs in
            lhs.hours != rhs.hours ? lhs.hours < rhs.hours : lhs.first > rhs.first
        }
        return best.map { ($0.link, $0.hours) }
    }
}
