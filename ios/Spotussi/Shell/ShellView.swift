import SpotussiKit
import SwiftUI

/// Five tabs, each with its own navigation stack. Search opens from the
/// toolbar: a sixth tab would push Library and Search under "More".
struct ShellView: View {
    nonisolated enum Destination: Hashable {
        case overview, top, stats, history, library
    }

    @Environment(InstanceStatus.self) private var status
    @Environment(NowPlayingModel.self) private var nowPlaying
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
        }
        .tabBarMinimizeBehavior(.onScrollDown)
        .nowPlayingAccessory(nowPlaying)
        // Polls only while the app is in front; leaving cancels it.
        .task(id: scenePhase == .active) {
            guard scenePhase == .active else { return }
            await nowPlaying.poll()
        }
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
