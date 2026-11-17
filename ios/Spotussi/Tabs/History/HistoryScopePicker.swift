import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Narrows History to one artist, album or track, found by name as the
/// web's History search box finds it (`/search`).
struct HistoryScopePicker: View {
    let pick: (EntityLink) -> Void

    @Environment(Session.self) private var session
    @Environment(\.dismiss) private var dismiss
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model = SearchModel()
    @State private var text = ""
    @State private var isFieldActive = false

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
            .navigationTitle("Filter by")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $text, isPresented: $isFieldActive, prompt: "Artist, album or track")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(role: .close) { dismiss() }
                }
            }
        }
        .task(id: text) { await model.search(text, client: client) }
        .onAppear { isFieldActive = true }
    }

    @ViewBuilder private var results: some View {
        if let groups = model.groups, !groups.isEmpty, let query, model.failed != query {
            VStack(alignment: .leading, spacing: 16) {
                ForEach(groups) { group in
                    VStack(alignment: .leading, spacing: 6) {
                        PanelTitle(SearchSheet.title(group.kind))
                        VStack(spacing: 2) {
                            ForEach(group.hits) { hit in
                                Button {
                                    pick(hit.link)
                                    dismiss()
                                } label: {
                                    SearchHitRow(hit: hit)
                                }
                                .buttonStyle(RowButtonStyle())
                                .accessibilityLabel(SearchHitRow.spokenLabel(hit))
                                .accessibilityValue(hit.link.kind.name)
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

    @ViewBuilder private var state: some View {
        if let query {
            if model.failed == query, model.answered != query {
                ContentUnavailableView("Couldn’t search.", systemImage: "exclamationmark.magnifyingglass")
            } else if model.answered == query, model.groups?.isEmpty == true {
                ContentUnavailableView("No results", systemImage: "magnifyingglass")
            } else if model.groups == nil, model.isSearching {
                ProgressView()
            }
        }
    }

    private var query: String? {
        CatalogSearch.query(text)
    }

    private var client: Client {
        session.client ?? .demo
    }
}
