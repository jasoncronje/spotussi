import SpotussiKit
import SwiftUI

/// One notice on a solid panel. Alerts (something has stopped working) get
/// a symbol and an outline in their colour; information sits quieter. The
/// symbol's shape and the words carry the meaning, not the colour alone.
struct NoticePanel: View {
    let notice: StatusNotice

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline, spacing: 12) {
                NoticeSymbol(kind: notice.kind)
                VStack(alignment: .leading, spacing: 3) {
                    Text(notice.title)
                        .font(.subheadline.weight(notice.isAlert ? .semibold : .medium))
                    Text(notice.message)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            if let progress = notice.progress {
                LampRow(progress: progress)
            }
        }
        .panel()
        .overlay {
            if notice.isAlert {
                RoundedRectangle(cornerRadius: 20)
                    .strokeBorder(NoticeSymbol.tint(for: notice.kind).opacity(0.45), lineWidth: 1)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(notice.accessibilityLabel)
    }
}

private struct NoticeSymbol: View {
    let kind: StatusNotice.Kind
    @ScaledMetric(relativeTo: .subheadline) private var dot = 8

    var body: some View {
        Group {
            switch kind {
            case .unreachable:
                Image(systemName: "wifi.exclamationmark")
            case .tokenRejected:
                Image(systemName: "lock.slash")
            case .reconnectSpotify:
                // The web's beacon: a lit lamp in a soft ring.
                Circle()
                    .fill(.lamp)
                    .frame(width: dot, height: dot)
                    .padding(dot * 0.4)
                    .background(Circle().fill(.lamp.opacity(0.22)))
            case .spotifyNotConnected:
                Circle()
                    .fill(.grille)
                    .frame(width: dot, height: dot)
            case .catalogProgress:
                Image(systemName: "music.note.list")
            }
        }
        .font(.subheadline.weight(.semibold))
        .foregroundStyle(Self.tint(for: kind))
        .accessibilityHidden(true)
    }

    static func tint(for kind: StatusNotice.Kind) -> AnyShapeStyle {
        switch kind {
        case .unreachable, .tokenRejected: AnyShapeStyle(.red)
        case .reconnectSpotify: AnyShapeStyle(.lamp)
        case .spotifyNotConnected, .catalogProgress: AnyShapeStyle(.secondary)
        }
    }
}
