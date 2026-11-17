import Foundation
import Observation

/// The range and measure the screens share, remembered across launches.
///
/// The range is one for the whole app, like the web's `range` that carries
/// across navigation. The measure is remembered per screen (D7), so Plays
/// left on Top stays on Top while Stats shows Time.
@MainActor @Observable
public final class ViewOptions {
    /// Screens with a Plays/Time switch: the web's `METRIC_VIEWS`.
    public enum Screen: String, CaseIterable, Hashable, Sendable {
        case overview, top, stats, artist, album, track, genre
    }

    public var range: StatsRange {
        didSet { defaults.set(range.storageValue, forKey: Self.rangeKey) }
    }

    private var measures: [Screen: Measure]

    @ObservationIgnored private let defaults: UserDefaults

    private static let rangeKey = "view.range"
    private static func measureKey(_ screen: Screen) -> String { "view.measure.\(screen.rawValue)" }

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        range = defaults.string(forKey: Self.rangeKey).flatMap(StatsRange.init(storageValue:))
            ?? .default
        var measures: [Screen: Measure] = [:]
        for screen in Screen.allCases {
            measures[screen] = defaults.string(forKey: Self.measureKey(screen))
                .flatMap(Measure.init(rawValue:))
        }
        self.measures = measures
    }

    /// Plays unless this screen was switched to Time.
    public func measure(for screen: Screen) -> Measure {
        measures[screen] ?? .plays
    }

    public func setMeasure(_ measure: Measure, for screen: Screen) {
        measures[screen] = measure
        defaults.set(measure.rawValue, forKey: Self.measureKey(screen))
    }
}
