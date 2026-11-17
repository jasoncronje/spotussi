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
            String(localized: "Nothing answered at that address.")
        case .pairing(.notSpotussi):
            String(localized: "Something answered, but it isn’t Spotussi.")
        case .pairing(.tokenRejected):
            String(localized: "The server didn’t accept the token.")
        case .pairing(.plainHTTPBlocked):
            String(localized: "iOS allows plain http only on your own network.")
        case .pairing(.cancelled):
            String(localized: "Pairing was called off.")
        case .pairing(.other(let status?)):
            String(localized: "Pairing didn’t finish (error \(status)).")
        case .pairing(.other(nil)):
            String(localized: "Pairing didn’t finish.")
        }
    }

    var advice: String {
        switch self {
        case .link(.notAPairingLink):
            String(localized: "Scan the code under Settings, API & MCP, Pair iPhone on the web.")
        case .link(.badAddress):
            String(
                localized:
                    "Type it as you would in a browser, such as stats.example.com or http://192.168.1.20:8080."
            )
        case .link(.badToken):
            String(
                localized:
                    "Tokens start with spt_. Copy it again from Settings, API & MCP, Pair iPhone on the web."
            )
        case .pairing(.unreachable):
            String(
                localized:
                    "Check the address, and that this iPhone is on the same network or VPN as the server."
            )
        case .pairing(.notSpotussi):
            String(localized: "Use the address you open Spotussi at in a browser.")
        case .pairing(.tokenRejected):
            String(
                localized:
                    "It may have been revoked or mistyped. Make a new one under Settings, API & MCP, Pair iPhone."
            )
        case .pairing(.plainHTTPBlocked):
            String(
                localized:
                    "Use https for a public name, or the server’s IP address or .local name with http."
            )
        case .pairing(.cancelled):
            String(localized: "Try again when you’re ready.")
        case .pairing(.other):
            String(
                localized:
                    "Try again in a moment. If it keeps happening, check the server’s logs.")
        }
    }
}
