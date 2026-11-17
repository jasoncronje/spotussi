import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The play log of one artist, album, track or genre, opened from its page
/// ("Plays in History"), with the same filters as History.
struct ScopedHistoryPage: View {
    let scope: EntityLink

    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @State private var model = HistoryModel()
    @State private var filter: HistoryFilter

    init(scope: EntityLink) {
        self.scope = scope
        _filter = State(initialValue: HistoryFilter(scope: scope))
    }

    var body: some View {
        let query = HistoryQuery(range: options.range, filter: filter)
        let range = options.range
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let log = model.log, model.answered == query {
                    CountLine(text: Text("\(log.total) plays"))
                }
                ActiveFilters(filter: $filter)
                HistoryList(model: model, query: query, clear: { filter = filter.cleared })
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 32)
        }
        .background(.ground)
        .navigationTitle("History")
        .navigationSubtitle(Text(scope.name))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                RangeMenu()
            }
            ToolbarItem(placement: .topBarTrailing) {
                HistoryFilterMenu(filter: $filter, devices: model.devices)
            }
        }
        .task(id: query) { await model.load(query, client: client) }
        .task(id: range) { await model.loadDevices(range, client: client) }
        .refreshable { [model, client] in
            await model.load(query, client: client, force: true)
        }
    }

    private var client: Client {
        session.client ?? .demo
    }
}

#Preview {
    PreviewHost(.demo) {
        NavigationStack {
            ScopedHistoryPage(scope: EntityLink(kind: .artist, id: "demo", name: "Iris Sakoti"))
                .routeDestinations()
        }
    }
}
