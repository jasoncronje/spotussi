import Foundation
import Observation
import SpotussiKit

/// One pairing attempt at a time, from whichever door it came in: the
/// scanner, the typed address and token, or a `spotussi://pair` link.
@Observable
final class PairingModel {
    nonisolated enum Phase: Equatable {
        case idle
        case connecting(Instance)
        case failed(PairingProblem)
    }

    private(set) var phase: Phase
    /// Goes up by one on every successful pairing: the success haptic and the
    /// shell's identity follow it, so a new token gets a fresh status model.
    private(set) var pairings = 0

    @ObservationIgnored private let session: Session
    @ObservationIgnored private var attempt: Task<Void, Never>?

    init(session: Session, phase: Phase = .idle) {
        self.session = session
        self.phase = phase
    }

    var isConnecting: Bool {
        if case .connecting = phase { true } else { false }
    }

    var connectingTo: Instance? {
        if case .connecting(let instance) = phase { instance } else { nil }
    }

    var problem: PairingProblem? {
        if case .failed(let problem) = phase { problem } else { nil }
    }

    // MARK: - Starting

    func pair(_ link: PairingLink) {
        attempt?.cancel()
        phase = .connecting(link.instance)
        attempt = Task {
            do throws(PairingError) {
                try await session.pair(link)
                guard !Task.isCancelled else { return }
                phase = .idle
                pairings += 1
            } catch {
                // A cancelled attempt has already been replaced or called off.
                guard !Task.isCancelled, error != .cancelled else { return }
                phase = .failed(.pairing(error))
            }
        }
    }

    /// Typed in by hand. A whole pairing link pasted into the address field
    /// works too.
    func pair(address: String, token: String) {
        if let pasted = URL(string: address.trimmingCharacters(in: .whitespacesAndNewlines)),
            let link = try? PairingLink(pasted)
        {
            pair(link)
            return
        }
        do throws(PairingLink.Failure) {
            pair(try PairingLink(address: address, token: token))
        } catch {
            phase = .failed(.link(error))
        }
    }

    func cancel() {
        attempt?.cancel()
        attempt = nil
        phase = .idle
    }

    func dismissProblem() {
        if case .failed = phase { phase = .idle }
    }

    // MARK: - Reading links

    /// A link opened from outside the app. Other `spotussi://` links belong
    /// to other screens, so they are not a problem here.
    func link(from url: URL) -> PairingLink? {
        do throws(PairingLink.Failure) {
            return try PairingLink(url)
        } catch .notAPairingLink {
            return nil
        } catch {
            phase = .failed(.link(error))
            return nil
        }
    }

    /// What the camera read, if it is a pairing code.
    func link(fromScanned payload: String) -> PairingLink? {
        guard let url = URL(string: payload) else { return nil }
        return try? PairingLink(url)
    }
}
