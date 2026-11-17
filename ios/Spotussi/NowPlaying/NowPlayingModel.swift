import Foundation
import Observation
import SpotussiAPI
import SpotussiKit

/// What the instance's Spotify account is playing, asked every 15 seconds
/// while the app is in front (the web's interval), backing off while the
/// server doesn't answer.
@Observable
final class NowPlayingModel {
    /// The playing or paused track; nil when nothing is playing.
    private(set) var nowPlaying: Components.Schemas.NowPlaying?

    @ObservationIgnored private let client: Client

    init(client: Client) {
        self.client = client
    }

    var track: Components.Schemas.TrackRef? { nowPlaying?.track }

    /// Asks now, then again after each wait, until the task is cancelled:
    /// the app leaving the foreground, or the connection changing.
    func poll() async {
        var schedule = PollSchedule()
        while !Task.isCancelled {
            do {
                nowPlaying = try await client.getNowPlaying().ok.body.json
                schedule.succeeded()
            } catch {
                guard !Task.isCancelled else { return }
                schedule.failed()
                if schedule.lastAnswerIsStale { nowPlaying = nil }
            }
            try? await Task.sleep(for: schedule.delay)
        }
    }
}
