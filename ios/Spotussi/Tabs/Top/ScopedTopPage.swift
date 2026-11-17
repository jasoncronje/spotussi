import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The whole ranking of one kind within an entity, opened from the "All"
/// link on its page: every track of an artist, every album of a genre.
/// Ranked by the entity page's measure.
struct ScopedTopPage: View {
    let kind: EntityKind
    let entity: EntityLink

    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @State private var model: RankedListModel
    @Namespace private var zoom

    init(kind: EntityKind, within entity: EntityLink) {
        self.kind = kind
        self.entity = entity
        _model = State(initialValue: RankedListModel(kind: kind, within: entity))
    }

    var body: some View {
        let query = self.query
        ScrollView {
            RankedListPanel(model: model, query: query, namespace: zoom)
                .padding(.horizontal, 16)
                .padding(.top, 8)
                .padding(.bottom, 32)
        }
        .background(.ground)
        .navigationTitle(title)
        .navigationSubtitle(Text(entity.name))
        .navigationBarTitleDisplayMode(.inline)
        .entityToolbar(screen: entity.kind.screen)
        .task(id: query) { await model.load(query, client: client) }
        .refreshable { [model, query, client] in
            await model.load(query, client: client, force: true)
        }
    }

    private var query: StatsQuery {
        StatsQuery(range: options.range, measure: options.measure(for: entity.kind.screen))
    }

    private var title: LocalizedStringKey {
        switch kind {
        case .track: "Top tracks"
        case .artist: "Top artists"
        case .album: "Top albums"
        case .genre: "Top genres"
        }
    }

    private var client: Client {
        session.client ?? .demo
    }
}

#Preview {
    PreviewHost(.demo) {
        NavigationStack {
            ScopedTopPage(kind: .track, within: EntityLink(kind: .artist, id: "demo", name: "Iris Sakoti"))
        }
    }
}
