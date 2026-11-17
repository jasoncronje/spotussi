import SpotussiKit
import SwiftUI

/// What every tab shares: its navigation stack and the pages rows open, the
/// status notices at the top of the scroll, the toolbar controls, and
/// refreshing on appear and on pull. A tab supplies its title and its
/// content.
struct TabPage<Content: View>: View {
    let title: LocalizedStringKey
    var subtitle: Text?
    let controls: TabControls
    /// A change scrolls back to the top: a new list replaces the old one.
    var scrollReset: AnyHashable?
    /// A change brings the view with this id to the top.
    var scrollTarget: ScrollTarget?
    /// The tab's own reload for pull-to-refresh; the status reloads anyway.
    var refresh: (@Sendable () async -> Void)?
    @ViewBuilder let content: Content

    @Environment(InstanceStatus.self) private var status
    @Environment(PairingModel.self) private var pairing
    @Environment(Navigator.self) private var navigator
    @Environment(\.tab) private var tab
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var position = ScrollPosition(edge: .top)
    /// The stack's pages outside the tab bar, where the navigator has none.
    @State private var localPath: [Route] = []

    var body: some View {
        NavigationStack(path: path) {
            ScrollViewReader { proxy in
                scroll
                    .onChange(of: scrollTarget) { _, target in
                        guard let target else { return }
                        proxy.scrollTo(target.id, anchor: .top)
                    }
            }
            .background(.ground)
            .navigationTitle(title)
            .tabToolbar(controls)
            .routeDestinations()
            .task { await status.refresh() }
            .refreshable { [status, refresh] in
                await status.refresh()
                await refresh?()
            }
        }
    }

    private var scroll: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // In the page, not the bar: a bar subtitle also shows under
                // the centred Plays/Time switch and pushes it up.
                if let subtitle {
                    subtitle
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .padding(.top, -12)
                }
                StatusNotices()
                content
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 32)
            .animation(reduceMotion ? nil : .smooth, value: status.notices)
            .animation(reduceMotion ? nil : .smooth, value: pairing.isConnecting)
        }
        .scrollPosition($position)
        .onChange(of: scrollReset) {
            position.scrollTo(edge: .top)
        }
    }

    private var path: Binding<[Route]> {
        guard let tab else { return $localPath }
        return navigator.path(for: tab)
    }
}

/// A view to bring to the top of a tab's scroll; a new `request` asks
/// again for the same view.
nonisolated struct ScrollTarget: Equatable {
    let id: String
    let request: Int
}

/// Which toolbar controls a tab shows.
struct TabControls {
    var range = false
    /// The screen whose Plays/Time choice the switch shows, if it has one.
    var measure: ViewOptions.Screen?

    static let settingsOnly = TabControls()
    static let rangeOnly = TabControls(range: true)
    static func rangeAndMeasure(_ screen: ViewOptions.Screen) -> TabControls {
        TabControls(range: true, measure: screen)
    }
}
