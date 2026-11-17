import SpotussiKit
import SwiftUI

/// Five tabs and search, each with its own navigation stack.
struct ShellView: View {
    nonisolated enum Destination: Hashable {
        case overview, top, stats, history, library, search
    }

    @Environment(InstanceStatus.self) private var status
    @Environment(ViewOptions.self) private var options
    @Environment(\.scenePhase) private var scenePhase
    @State private var selection = Destination.overview

    var body: some View {
        TabView(selection: $selection) {
            Tab("Overview", systemImage: "gauge.with.dots.needle.33percent", value: .overview) {
                OverviewTab()
            }
            Tab("Top", systemImage: "list.number", value: .top) {
                TopTab()
            }
            Tab("Stats", systemImage: "chart.bar.xaxis", value: .stats) {
                StatsTab()
            }
            Tab("History", systemImage: "clock.arrow.circlepath", value: .history) {
                HistoryTab()
            }
            Tab("Library", systemImage: "books.vertical", value: .library) {
                LibraryTab()
            }
            Tab(value: .search, role: .search) {
                SearchTab()
            }
        }
        .tabBarMinimizeBehavior(.onScrollDown)
        // Here rather than on each tab, which stay alive in the background
        // and would each give a tick.
        .sensoryFeedback(.selection, trigger: options.range)
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active else { return }
            Task { await status.refresh() }
        }
    }
}

#Preview("Demo") {
    PreviewHost(.demo) { ShellView() }
}
