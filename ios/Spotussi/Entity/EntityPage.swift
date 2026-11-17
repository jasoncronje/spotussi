import SpotussiAPI
import SpotussiKit
import SwiftUI

/// An artist, album, track or genre page (D14), the same layout for all
/// four, as the web's entity view: who or what it is, where it ranks in the
/// range, its totals, listening over the range, what inside it was played
/// most, and where it was played from. Everything below the head is
/// narrowed to the entity, for the shared range and this kind of page's
/// measure.
struct EntityPage: View {
    let link: EntityLink

    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @State private var model: EntityModel
    @Namespace private var zoom

    init(link: EntityLink) {
        self.link = link
        _model = State(initialValue: EntityModel(link: link))
    }

    var body: some View {
        let query = self.query
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                EntityHeader(link: link, detail: model.detail.value)
                sections(query)
                    .padding(.horizontal, 16)
            }
            .padding(.bottom, 32)
        }
        .background(.ground)
        // The soft edge lets the artwork show through under the bar, where
        // iOS 27's default hard edge would cover it.
        .scrollEdgeEffectStyle(.soft, for: .top)
        .navigationTitle(model.detail.value?.name ?? link.name)
        .navigationBarTitleDisplayMode(.inline)
        // The bar holds back, range, Plays/Time and Open in Spotify: a title
        // there is clipped to a letter, and the name leads the page anyway.
        .toolbar(removing: .title)
        .entityToolbar(screen: link.kind.screen, spotifyURI: model.detail.value?.spotifyURI)
        .task(id: query) { await model.load(query, client: client) }
        .refreshable { [model, query, client] in
            await model.load(query, client: client)
        }
    }

    @ViewBuilder private func sections(_ query: StatsQuery) -> some View {
        if model.isNotFound, model.detail.current(for: query) == nil {
            Text("Not in your library.")
                .foregroundStyle(.secondary)
                .panel()
        } else {
            if model.detail.value?.isExcluded == true {
                Label("Excluded from stats", systemImage: "eye.slash")
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(.secondary)
                    .panel()
            }
            LoadedSection(model.detail, for: query) { detail, isFailed in
                if isFailed {
                    SectionFailure().panel()
                } else {
                    EntityFigures(kind: link.kind, detail: detail, measure: query.measure)
                }
            }
            if let detail = model.detail.current(for: query), !detail.isListened {
                NotPlayedPanel(hasPlays: detail.allTime.plays > 0)
            } else {
                listening(query)
            }
        }
    }

    @ViewBuilder private func listening(_ query: StatsQuery) -> some View {
        LoadedSection(model.series, for: query) { series, isFailed in
            PlaysByDayPanel(series: series, isFailed: isFailed, screen: link.kind.screen)
        }
        ForEach(model.listed, id: \.self) { listed in
            LoadedSection(model.list(listed.kind), for: query) { page, isFailed in
                if isShown(listed.kind, page: page) {
                    TopListPanel(
                        title: title(listed.kind), kind: listed.kind, entries: page.map { entries($0, listed.kind) },
                        measure: query.measure, isFailed: isFailed, namespace: zoom, placeholderCount: listed.count,
                        all: page.map { TopListPanel.AllLink(route: .ranked(listed.kind, within: entity), total: $0.total) })
                }
            }
        }
        if link.kind == .track {
            LoadedSection(model.plays, for: query) { plays, isFailed in
                RecentPlaysPanel(title: "Latest plays", plays: plays, failed: isFailed)
            }
        }
        LoadedSection(model.contexts, for: query) { contexts, isFailed in
            PlayContextsPanel(contexts: contexts, measure: query.measure, isFailed: isFailed)
        }
    }

    /// Most albums have one or two artists, named in the head; the ranked
    /// list of artists on its tracks is for compilations and soundtracks,
    /// as on the web.
    private func isShown(_ kind: EntityKind, page: TopPage?) -> Bool {
        guard link.kind == .album, kind == .artist else { return true }
        return (page?.total ?? 0) >= 3
    }

    /// An album's tracks are all on the album: their artists would repeat
    /// the head.
    private func entries(_ page: TopPage, _ kind: EntityKind) -> [TopEntry] {
        guard link.kind == .album, kind == .track else { return page.entries }
        return page.entries.map { entry in
            var entry = entry
            entry.detail = nil
            return entry
        }
    }

    private func title(_ kind: EntityKind) -> LocalizedStringKey {
        switch (link.kind, kind) {
        case (.album, .track): "Tracks"
        case (.album, .artist): "Artists"
        case (_, .track): "Top tracks"
        case (_, .artist): "Top artists"
        case (_, .album): "Top albums"
        case (_, .genre): "Top genres"
        }
    }

    /// The entity as loaded, for the "All" pages' subtitles.
    private var entity: EntityLink {
        model.detail.value?.link ?? link
    }

    private var query: StatsQuery {
        StatsQuery(range: options.range, measure: options.measure(for: link.kind.screen))
    }

    private var client: Client {
        session.client ?? .demo
    }
}

/// Nothing in the range: say so, and offer all time when there was
/// listening before.
private struct NotPlayedPanel: View {
    let hasPlays: Bool
    @Environment(ViewOptions.self) private var options

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Not played in this range")
                .font(.headline)
            if hasPlays {
                Button("Show all time") {
                    options.range = .preset(.all)
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
                // Neutral: the range's lamp is the page's one green control.
                .tint(.primary)
            }
        }
        .panel()
    }
}

#Preview("Artist") {
    PreviewHost(.demo) {
        NavigationStack {
            EntityPage(link: EntityLink(kind: .artist, id: "demo", name: "Iris Sakoti"))
                .routeDestinations()
        }
    }
}

#Preview("Album, light") {
    PreviewHost(.demo) {
        NavigationStack {
            EntityPage(link: EntityLink(kind: .album, id: "demo", name: "The Honest Paradise"))
                .routeDestinations()
        }
    }
    .preferredColorScheme(.light)
}

#Preview("Track, largest text") {
    PreviewHost(.demo) {
        NavigationStack {
            EntityPage(link: EntityLink(kind: .track, id: "demo", name: "The Honest Paradise"))
                .routeDestinations()
        }
    }
    .dynamicTypeSize(.accessibility5)
}

#Preview("Genre") {
    PreviewHost(.demo) {
        NavigationStack {
            EntityPage(link: EntityLink(kind: .genre, id: "demo", name: "modern pop"))
                .routeDestinations()
        }
    }
}
