import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Search, opened as a sheet from any tab's toolbar: artists, albums and
/// tracks by name, ordered by all-time plays. Results open their pages in
/// the sheet's own stack.
struct SearchSheet: View {
    @Environment(Session.self) private var session
    @Environment(\.dismiss) private var dismiss
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model = SearchModel()
    @State private var text: String
    @State private var isFieldActive = false
    @Namespace private var zoom

    init(text: String = "") {
        _text = State(initialValue: text)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                results
                    .padding(.horizontal, 16)
                    .padding(.top, 8)
                    .padding(.bottom, 32)
            }
            .scrollDismissesKeyboard(.immediately)
            .background(.ground)
            .overlay { state }
            .navigationTitle("Search")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $text, isPresented: $isFieldActive, prompt: "Tracks, artists, albums")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(role: .close) { dismiss() }
                }
            }
            .routeDestinations()
        }
        .task(id: text) { await model.search(text, client: client) }
        .onAppear { isFieldActive = true }
    }

    @ViewBuilder private var results: some View {
        if let groups = model.groups, !groups.isEmpty, let query, model.failed != query {
            VStack(alignment: .leading, spacing: 16) {
                ForEach(groups) { group in
                    VStack(alignment: .leading, spacing: 6) {
                        PanelTitle(Self.title(group.kind))
                        VStack(spacing: 2) {
                            ForEach(group.hits) { hit in
                                SearchHitLink(hit: hit, namespace: zoom)
                            }
                        }
                    }
                    .panel()
                }
            }
            .opacity(model.answered == query ? 1 : 0.55)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: model.answered == query)
        }
    }

    /// Before typing, no results, a failure, or the first search on its way.
    @ViewBuilder private var state: some View {
        if let query {
            if model.failed == query, model.answered != query {
                ContentUnavailableView("Couldn’t search.", systemImage: "exclamationmark.magnifyingglass")
            } else if model.answered == query, model.groups?.isEmpty == true {
                ContentUnavailableView("No results", systemImage: "magnifyingglass")
            } else if model.groups == nil, model.isSearching {
                ProgressView()
            }
        } else {
            ContentUnavailableView("Search what you’ve played", systemImage: "magnifyingglass")
        }
    }

    private var query: String? {
        CatalogSearch.query(text)
    }

    private static func title(_ kind: EntityKind) -> LocalizedStringKey {
        switch kind {
        case .artist: "Artists"
        case .album: "Albums"
        case .track: "Tracks"
        case .genre: "Genres"
        }
    }

    private var client: Client {
        session.client ?? .demo
    }
}

/// One result: artwork, name and artists. Opens its page.
private struct SearchHitLink: View {
    let hit: SearchHit
    let namespace: Namespace.ID
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let zoom = ZoomOrigin(id: hit.id, namespace: namespace)
        RouteLink(
            route: .entity(hit.link, zoom: zoom),
            accessibilityLabel: [hit.link.name, hit.detail].compactMap(\.self).joined(separator: ", "),
            accessibilityValue: hit.link.kind.name
        ) {
            let layout =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
                : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
            layout {
                ArtworkImage(url: hit.imageURL, side: 40, symbol: hit.link.kind.symbol, isRound: hit.link.kind == .artist)
                    .zoomSource(zoom)
                VStack(alignment: .leading, spacing: 2) {
                    Text(hit.link.name)
                        .font(.body.weight(.medium))
                    if let detail = hit.detail {
                        Text(detail)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(.vertical, 6)
            .frame(minHeight: 44)
        }
    }
}

#Preview("Empty") {
    PreviewHost(.demo) { SearchSheet() }
}

#Preview("Results") {
    PreviewHost(.demo) { SearchSheet(text: "iris") }
}

#Preview("Results, largest text") {
    PreviewHost(.demo) { SearchSheet(text: "iris") }
        .dynamicTypeSize(.accessibility5)
}
