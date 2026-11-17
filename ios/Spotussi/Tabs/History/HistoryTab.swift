import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Every play in the range, newest first, with the web's filters; and the
/// longest sessions, behind a switch under the title (Sessions lives
/// inside History on the phone). Each part keeps its place when switching.
struct HistoryTab: View {
    nonisolated enum Part: Hashable, Sendable {
        case plays, sessions
    }

    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    @State private var part = Part.plays
    @State private var filter = HistoryFilter()
    @State private var history = HistoryModel()
    @State private var sessions = SessionsModel()
    @State private var isPickingScope = false

    var body: some View {
        let query = HistoryQuery(range: options.range, filter: filter)
        let range = options.range
        let part = part
        TabPage(
            title: "History", subtitle: Text(options.range.longLabel), controls: .rangeOnly,
            scrollReset: Shown(part: part, query: query),
            refresh: { [history, sessions, client] in
                switch part {
                case .plays: await history.load(query, client: client, force: true)
                case .sessions: await sessions.load(range, client: client, force: true)
                }
            }
        ) {
            PartPicker(selection: $part)
                // On the one view, not the list's: a toolbar on a group of
                // views is added once for each of them.
                .toolbar {
                    if part == .plays {
                        ToolbarItem(placement: .topBarTrailing) {
                            HistoryFilterMenu(
                                filter: $filter, devices: history.devices, pickScope: { isPickingScope = true })
                        }
                        ToolbarSpacer(.fixed, placement: .topBarTrailing)
                    }
                }
            count
            switch part {
            case .plays:
                ActiveFilters(filter: $filter, scopeIsFilter: true)
                HistoryList(model: history, query: query, scopeIsFilter: true, clear: { filter = HistoryFilter() })
            case .sessions:
                SessionsList(model: sessions, range: range)
            }
        }
        .task(id: Shown(part: part, query: query)) {
            switch part {
            case .plays: await history.load(query, client: client)
            case .sessions: await sessions.load(range, client: client)
            }
        }
        .task(id: range) { await history.loadDevices(range, client: client) }
        .sheet(isPresented: $isPickingScope) {
            HistoryScopePicker { filter.scope = $0 }
        }
    }

    /// "2,348 plays", "102 sessions · 15 min gap", once they answer the
    /// current range.
    @ViewBuilder private var count: some View {
        let range = options.range
        switch part {
        case .plays:
            if let log = history.log, history.answered == HistoryQuery(range: range, filter: filter) {
                CountLine(text: Text("\(log.total) plays"))
            }
        case .sessions:
            if let list = sessions.list, sessions.answered == range, list.total > 0 {
                CountLine(text: Text("\(list.total) sessions · \(list.gapMinutes) min gap"))
            }
        }
    }

    /// What the screen shows; a change loads it and scrolls to the top.
    private nonisolated struct Shown: Hashable {
        let part: Part
        let query: HistoryQuery
    }

    private var client: Client {
        session.client ?? .demo
    }
}

/// Plays or Sessions.
private struct PartPicker: View {
    @Binding var selection: HistoryTab.Part

    var body: some View {
        Picker("Show", selection: $selection) {
            Text("Plays").tag(HistoryTab.Part.plays)
            Text("Sessions").tag(HistoryTab.Part.sessions)
        }
        .pickerStyle(.segmented)
        .labelsHidden()
    }
}

/// A count above a list, as on Top.
struct CountLine: View {
    let text: Text

    var body: some View {
        text
            .font(.footnote.monospacedDigit())
            .foregroundStyle(.secondary)
            .padding(.horizontal, 4)
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { HistoryTab() }
}

#Preview("Largest text") {
    PreviewHost(.demo) { HistoryTab() }
        .dynamicTypeSize(.accessibility5)
}

#Preview("Light") {
    PreviewHost(.demo) { HistoryTab() }
        .preferredColorScheme(.light)
}
