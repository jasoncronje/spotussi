import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// One session and its rows in order.
@Observable
final class SessionModel {
    let id: String
    private(set) var session: ListeningSession?
    private(set) var plays: [RecentPlay] = []
    /// Sessions are worked out again when the gap or the history changes,
    /// so one opened from an old list can be gone.
    private(set) var isNotFound = false
    private(set) var isFailed = false

    init(id: String) {
        self.id = id
    }

    func load(client: Client) async {
        do {
            let (session, plays) = try await ListeningSession.load(id: id, client: client)
            self.session = session
            self.plays = plays
            isNotFound = false
            isFailed = false
        } catch {
            guard !Task.isCancelled else { return }
            isNotFound = (error as? EntityLoadError) == .notFound
            isFailed = true
        }
    }
}
