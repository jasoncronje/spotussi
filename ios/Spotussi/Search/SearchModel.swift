import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// Catalog search as the web runs it: from two characters, a short pause
/// after the last keystroke, five results a kind. Each keystroke cancels
/// the search before it; the last results stay (dimmed) until the next
/// ones arrive.
@Observable
final class SearchModel {
    private(set) var groups: [SearchGroup]?
    /// The query `groups` answers.
    private(set) var answered: String?
    /// The query whose search failed.
    private(set) var failed: String?
    private(set) var isSearching = false

    func search(_ text: String, client: Client) async {
        guard let query = CatalogSearch.query(text) else {
            groups = nil
            answered = nil
            failed = nil
            isSearching = false
            return
        }
        guard query != answered else {
            isSearching = false
            return
        }
        isSearching = true
        // Cancelled by the next keystroke before the pause is over.
        do { try await Task.sleep(for: CatalogSearch.debounce) } catch { return }
        do {
            let results = try await client.search(query: .init(q: query, limit: CatalogSearch.limit)).ok.body.json
            try Task.checkCancellation()
            groups = SearchGroup.list(results)
            answered = query
            failed = nil
        } catch {
            guard !Task.isCancelled else { return }
            failed = query
        }
        isSearching = false
    }
}
