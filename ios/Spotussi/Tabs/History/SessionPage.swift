import SpotussiAPI
import SpotussiKit
import SwiftUI

/// One listening session, as the web's session page: how long it ran and
/// how much of it was listened, its plays and skips, its rows on a tape at
/// their real times, the artists played most, and every row in order,
/// each opening its track.
struct SessionPage: View {
    let link: SessionLink

    @Environment(Session.self) private var session
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var model: SessionModel
    @Namespace private var zoom

    init(link: SessionLink) {
        self.link = link
        _model = State(initialValue: SessionModel(id: link.id))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if model.isNotFound {
                    Text("Session not found.")
                        .foregroundStyle(.secondary)
                        .panel()
                } else if model.isFailed, model.session == nil {
                    SectionFailure().panel()
                } else {
                    content
                }
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 32)
        }
        .background(.ground)
        .navigationTitle(Figures.day(model.session?.startedAt ?? link.startedAt, now: .now))
        .navigationSubtitle(Text(subtitle))
        .navigationBarTitleDisplayMode(.inline)
        .task { await model.load(client: client) }
        .refreshable { [model, client] in
            await model.load(client: client)
        }
    }

    @ViewBuilder private var content: some View {
        let loaded = model.session
        figures(loaded)
            .placeholder(loaded == nil)
        if let loaded {
            SessionTapePanel(session: loaded, tape: SessionTape(session: loaded, plays: model.plays))
            if !loaded.topArtists.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Mostly")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .accessibilityAddTraits(.isHeader)
                    FlowLayout {
                        ForEach(loaded.topArtists, id: \.self) { artist in
                            EntityChip(link: artist)
                        }
                    }
                }
            }
            plays
        }
    }

    private func figures(_ loaded: ListeningSession?) -> some View {
        let columns = Array(
            repeating: GridItem(.flexible(), spacing: 12, alignment: .top),
            count: dynamicTypeSize.isAccessibilitySize ? 1 : 2)
        return LazyVGrid(columns: columns, spacing: 12) {
            Readout(
                label: "Length", value: Figures.duration(milliseconds: loaded?.duration ?? 0),
                spokenValue: Figures.duration(milliseconds: loaded?.duration ?? 0, width: .wide))
            Readout(
                label: "Listened", value: Figures.duration(milliseconds: loaded?.listened ?? 0),
                spokenValue: Figures.duration(milliseconds: loaded?.listened ?? 0, width: .wide))
            Readout(label: "Plays", value: Figures.count(loaded?.plays ?? 0))
            Readout(label: "Skipped", value: Figures.count(loaded?.skips ?? 0))
        }
    }

    private var plays: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                PanelTitle("In order")
                Spacer(minLength: 12)
                Text(Figures.count(model.plays.count))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
            }
            VStack(spacing: 2) {
                ForEach(model.plays) { play in
                    let origin = ZoomOrigin(id: "session:\(play.id)", namespace: zoom)
                    RouteLink(
                        route: .entity(EntityLink(kind: .track, id: play.trackID, name: play.name), zoom: origin),
                        accessibilityLabel: PlayRow.spokenLabel(play),
                        accessibilityValue: PlayRow.spokenValue(play, now: .now, style: .clock)
                    ) {
                        PlayRow(play: play, now: .now, style: .clock, zoom: origin)
                    }
                }
            }
        }
        .panel()
    }

    /// "2:46 – 4:26 PM", or the start until the session is in.
    private var subtitle: String {
        guard let loaded = model.session else { return Figures.clockTime(link.startedAt) }
        return Figures.timeSpan(loaded.startedAt, loaded.endedAt)
    }

    private var client: Client {
        session.client ?? .demo
    }
}

#Preview {
    PreviewHost(.demo) {
        NavigationStack {
            SessionPage(link: SessionLink(id: "49024", startedAt: Date(milliseconds: 1_792_939_587_000)))
                .routeDestinations()
        }
    }
}

#Preview("Largest text") {
    PreviewHost(.demo) {
        NavigationStack {
            SessionPage(link: SessionLink(id: "49024", startedAt: Date(milliseconds: 1_792_939_587_000)))
                .routeDestinations()
        }
    }
    .dynamicTypeSize(.accessibility5)
}
