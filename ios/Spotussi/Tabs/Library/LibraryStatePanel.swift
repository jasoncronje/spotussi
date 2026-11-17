import SpotussiKit
import SwiftUI

/// Why the Library has nothing to show, in one line, and the first sync's
/// progress while it runs. Read-only: every fix is on the web.
struct LibraryStatePanel: View {
    let empty: LibraryCondition.Empty
    let syncing: LibraryCondition.Syncing?

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline, spacing: 12) {
                symbol
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
                Text(Self.message(empty))
                    .font(.subheadline.weight(.medium))
                    .frame(maxWidth: .infinity, alignment: .leading)
                if empty == .firstSync, let fraction = syncing?.fraction {
                    Text(Figures.percent(fraction))
                        .font(.subheadline.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            if empty == .firstSync, let syncing, syncing.fraction != nil {
                LampRow(lit: syncing.litLamps(of: 24))
            }
        }
        .panel()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.message(empty))
        .accessibilityValue(syncing?.fraction.map { Figures.percent($0) } ?? "")
    }

    @ViewBuilder private var symbol: some View {
        switch empty {
        case .firstSync:
            if syncing != nil, syncing?.fraction == nil {
                ProgressView()
                    .controlSize(.small)
            } else {
                Image(systemName: "arrow.triangle.2.circlepath")
            }
        case .needsScope: Image(systemName: "lock")
        case .notConnected: Image(systemName: "link")
        case .syncOff: Image(systemName: "pause.circle")
        case .nothing: Image(systemName: "books.vertical")
        }
    }

    static func message(_ empty: LibraryCondition.Empty) -> String {
        switch empty {
        case .firstSync: String(localized: "First sync running.")
        case .needsScope: needsScope
        case .notConnected: String(localized: "Spotify isn’t connected.")
        case .syncOff: String(localized: "Library sync is off.")
        case .nothing: String(localized: "Nothing saved yet.")
        }
    }

    static var needsScope: String {
        String(localized: "Library access missing; reconnect Spotify on the web.")
    }
}

/// Playlists synced but saved songs and albums lack the permission: the
/// page shows what it has, with this line above it.
struct LibraryScopeNotice: View {
    var body: some View {
        Label(LibraryStatePanel.needsScope, systemImage: "lock")
            .font(.subheadline)
            .foregroundStyle(.secondary)
            .panel()
            .accessibilityElement(children: .combine)
    }
}
