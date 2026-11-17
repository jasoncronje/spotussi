import Foundation
import SpotussiAPI

/// Songs saved and removed over the years, as the web's saves chart: one
/// column of lamps per month, quarter or year, whichever keeps columns wide
/// enough, lit to the busiest column, with removals as dim lamps under
/// the line.
public struct SavesTimeline: Hashable, Sendable {
    /// One month, quarter or year.
    public struct Column: Identifiable, Hashable, Sendable {
        /// "2024-0" for the first group of 2024.
        public let id: String
        public let year: Int
        /// 1...12, the group's first month.
        public let month: Int
        /// 1, 3 or 12.
        public let months: Int
        public fileprivate(set) var saved: Int
        public fileprivate(set) var removed: Int
    }

    struct Month: Hashable, Sendable {
        let year: Int
        let month: Int
        let saved: Int
        let removed: Int
    }

    let monthly: [Month]

    /// Lamps above the line, and below it for removals.
    public static let rows = 9
    public static let removalRows = 3

    /// From `/library/timeline?bucket=month`. Points whose key isn't a
    /// month (`2024-03`) are skipped.
    public init(_ timeline: Components.Schemas.LibraryTimeline) {
        monthly = timeline.points.compactMap { point in
            let parts = point.key.split(separator: "-")
            guard parts.count >= 2, let year = Int(parts[0]), let month = Int(parts[1]), (1...12).contains(month)
            else { return nil }
            return Month(year: year, month: month, saved: point.tracksSaved, removed: point.tracksRemoved)
        }
    }

    /// Every month since the first save, as the web asks for it.
    public static func load(client: Client) async throws -> SavesTimeline {
        SavesTimeline(try await client.getLibraryTimeline(query: .init(range: .all, bucket: .month)).ok.body.json)
    }

    public var isEmpty: Bool { monthly.allSatisfy { $0.saved == 0 && $0.removed == 0 } }
    public var saved: Int { monthly.reduce(0) { $0 + $1.saved } }
    public var removed: Int { monthly.reduce(0) { $0 + $1.removed } }

    /// Months grouped `months` at a time (1, 3 or 12).
    public func columns(months: Int) -> [Column] {
        var columns: [Column] = []
        for point in monthly {
            let group = (point.month - 1) / months
            let id = "\(point.year)-\(group)"
            if columns.last?.id == id {
                columns[columns.count - 1].saved += point.saved
                columns[columns.count - 1].removed += point.removed
            } else {
                columns.append(
                    Column(
                        id: id, year: point.year, month: group * months + 1, months: months, saved: point.saved,
                        removed: point.removed))
            }
        }
        return columns
    }

    /// The finest grouping whose columns are at least `pitch` wide in
    /// `width`; years when nothing finer fits.
    public func columns(fitting width: Double, pitch: Double = 7) -> [Column] {
        for months in [1, 3] {
            let columns = columns(months: months)
            if width / Double(max(1, columns.count)) >= pitch { return columns }
        }
        return columns(months: 12)
    }

    /// How brightly each lamp above the line is lit, bottom up: 0 unlit,
    /// else 0.35...1, the top one part-lit, as the web draws them.
    public static func lamps(_ column: Column, peak: Int) -> [Double] {
        guard column.saved > 0 else { return Array(repeating: 0, count: rows) }
        let lit = Double(column.saved) / Double(max(1, peak)) * Double(rows)
        return (0..<rows).map { row in
            let share = min(1, max(0, lit - Double(row)))
            return share > 0 ? max(share, 0.35) : 0
        }
    }

    /// Dim lamps under the line: at least one for any removal.
    public static func removalLamps(_ column: Column, peak: Int) -> Int {
        guard column.removed > 0 else { return 0 }
        let scaled = Double(column.removed) / Double(max(1, peak)) * Double(removalRows)
        return max(1, Int(scaled.rounded()))
    }

    /// Songs saved and removed per year, for VoiceOver.
    public var years: [Column] { columns(months: 12) }
}
