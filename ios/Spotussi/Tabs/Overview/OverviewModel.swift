import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// The Overview's summary for the shared range and its measure. Until I2
/// brings the full Overview, this is the figures the I0 demo screen showed.
@Observable
final class OverviewModel {
    /// The last summary that loaded; kept while the next one loads, so a
    /// range change updates the figures instead of blanking them.
    private(set) var summary: Components.Schemas.OverviewSummary?
    private(set) var didFail = false

    /// What the summary depends on; a change re-queries.
    nonisolated struct Query: Equatable, Sendable {
        let range: StatsRange
        let measure: Measure
    }

    func load(_ query: Query, client: Client) async {
        let range = query.range.query
        do {
            let output = try await client.getOverviewSummary(
                query: .init(
                    range: range.range, from: range.from, to: range.to,
                    metric: query.measure.metric))
            summary = try output.ok.body.json
            didFail = false
        } catch {
            // Leaving the screen or changing the range again cancels a load;
            // that isn't a failure. Unreachable servers show in the notices.
            guard !Task.isCancelled else { return }
            didFail = true
        }
    }
}
