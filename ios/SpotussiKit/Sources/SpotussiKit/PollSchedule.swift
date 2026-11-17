/// How often to ask what is playing: every 15 seconds, the web's interval,
/// doubling after each failure in a row up to two minutes so an
/// unreachable server isn't asked four times a minute.
public struct PollSchedule: Hashable, Sendable {
    public static let interval = Duration.seconds(15)
    public static let longest = Duration.seconds(120)

    public private(set) var failures = 0

    public init() {}

    public mutating func succeeded() { failures = 0 }
    public mutating func failed() { failures += 1 }

    /// The wait before the next request.
    public var delay: Duration {
        min(Self.interval * (1 << min(failures, 4)), Self.longest)
    }

    /// After two failures in a row the last answer is too old to show.
    public var lastAnswerIsStale: Bool { failures >= 2 }
}
