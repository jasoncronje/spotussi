import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Top tracks, artists, albums and genres for the range, ranked by this
/// screen's measure, each row moving against the period before. Each kind
/// keeps its own list, so switching back finds it as it was.
struct TopTab: View {
    @Environment(Session.self) private var session
    @Environment(ViewOptions.self) private var options
    /// Remembered while the app runs.
    @State private var kind = EntityKind.track
    @State private var lists = Dictionary(
        uniqueKeysWithValues: EntityKind.allCases.map { ($0, RankedListModel(kind: $0)) })
    @Namespace private var zoom

    var body: some View {
        let model = lists[kind] ?? RankedListModel(kind: kind)
        let query = self.query
        TabPage(
            title: "Top", subtitle: subtitle, controls: .rangeAndMeasure(.top),
            scrollReset: Shown(kind: kind, query: query),
            refresh: { [model, query, client] in
                await model.load(query, client: client, force: true)
            }
        ) {
            KindPicker(selection: $kind)
            if let total = model.list?.total, model.answered == query, total > 0 {
                Text(KindPicker.count(total, of: kind))
                    .font(.footnote.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 4)
            }
            RankedListPanel(model: model, query: query, namespace: zoom)
        }
        .task(id: Shown(kind: kind, query: query)) {
            await model.load(query, client: client)
        }
    }

    /// What the screen shows; a change loads it and scrolls to the top.
    private nonisolated struct Shown: Hashable {
        let kind: EntityKind
        let query: StatsQuery
    }

    private var query: StatsQuery {
        StatsQuery(range: options.range, measure: options.measure(for: .top))
    }

    private var subtitle: Text {
        options.measure(for: .top) == .plays
            ? Text("\(options.range.longLabel) · by plays")
            : Text("\(options.range.longLabel) · by time")
    }

    private var client: Client {
        session.client ?? .demo
    }
}

/// Tracks, Artists, Albums, Genres: segmented while the four labels fit,
/// a menu at the largest text sizes.
private struct KindPicker: View {
    @Binding var selection: EntityKind

    /// The web's order is artists first; the phone opens on tracks, as in
    /// the mockups.
    private static let order: [EntityKind] = [.track, .artist, .album, .genre]

    var body: some View {
        ViewThatFits(in: .horizontal) {
            picker.pickerStyle(.segmented).fixedSize()
            picker.pickerStyle(.menu)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var picker: some View {
        Picker("Kind", selection: $selection) {
            ForEach(Self.order, id: \.self) { kind in
                Text(Self.label(kind)).tag(kind)
            }
        }
    }

    static func label(_ kind: EntityKind) -> LocalizedStringKey {
        switch kind {
        case .track: "Tracks"
        case .artist: "Artists"
        case .album: "Albums"
        case .genre: "Genres"
        }
    }

    /// "750 tracks"
    static func count(_ count: Int, of kind: EntityKind) -> LocalizedStringKey {
        switch kind {
        case .track: "\(count) tracks"
        case .artist: "\(count) artists"
        case .album: "\(count) albums"
        case .genre: "\(count) genres"
        }
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { TopTab() }
}

#Preview("Largest text") {
    PreviewHost(.demo) { TopTab() }
        .dynamicTypeSize(.accessibility5)
}

#Preview("Light") {
    PreviewHost(.demo) { TopTab() }
        .preferredColorScheme(.light)
}
