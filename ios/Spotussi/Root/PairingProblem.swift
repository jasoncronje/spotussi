import Foundation
import SpotussiKit

/// Why pairing didn't work, in words that say what to do next.
nonisolated enum PairingProblem: Equatable {
    case link(PairingLink.Failure)
    case pairing(PairingError)

    var title: String {
        switch self {
        case .link(.notAPairingLink):
            String(localized: "That isn’t a Spotussi pairing code.")
        case .link(.badAddress):
            String(localized: "That address can’t be used.")
        case .link(.badToken):
            String(localized: "That doesn’t look like a Spotussi token.")
        case .pairing(.unreachable):
            String(localized: "Can’t reach that address.")
        case .pairing(.notSpotussi):
            String(localized: "That isn’t a Spotussi server.")
        case .pairing(.tokenRejected):
            String(localized: "Token not accepted.")
        case .pairing(.plainHTTPBlocked):
            String(localized: "iOS allows plain http only on your own network.")
        case .pairing(.cancelled):
            String(localized: "Pairing cancelled.")
        case .pairing(.other(let status?)):
            String(localized: "Pairing didn’t finish (error \(status)).")
        case .pairing(.other(nil)):
            String(localized: "Pairing didn’t finish.")
        }
    }

    var advice: String {
        switch self {
        case .link(.notAPairingLink):
            String(localized: "Scan the code from Settings, API & MCP.")
        case .link(.badAddress):
            String(
                localized:
                    "Use the address from your browser, like stats.example.com."
            )
        case .link(.badToken):
            String(
                localized:
                    "Tokens start with spt_. Copy it again."
            )
        case .pairing(.unreachable):
            String(
                localized:
                    "Check the address, and that you’re on the same network or VPN."
            )
        case .pairing(.notSpotussi):
            String(localized: "Use the address from your browser.")
        case .pairing(.tokenRejected):
            String(
                localized:
                    "Make a new one in Settings, API & MCP."
            )
        case .pairing(.plainHTTPBlocked):
            String(
                localized:
                    "Use https, or the server’s IP address."
            )
        case .pairing(.cancelled):
            String(localized: "Try again any time.")
        case .pairing(.other):
            String(
                localized:
                    "Try again. If it keeps failing, check the server’s logs.")
        }
    }
}
