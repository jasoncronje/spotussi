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
    /// The tab's own reload for pull-to-refresh; the status reloads anyway.
    var refresh: (@Sendable () async -> Void)?
    @ViewBuilder let content: Content

    @Environment(InstanceStatus.self) private var status
    @Environment(PairingModel.self) private var pairing
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var position = ScrollPosition(edge: .top)

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
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
            .background(.ground)
            .navigationTitle(title)
            .modifier(Subtitle(text: subtitle))
            .tabToolbar(controls)
            .routeDestinations()
            .task { await status.refresh() }
            .refreshable { [status, refresh] in
                await status.refresh()
                await refresh?()
            }
        }
    }
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

private struct Subtitle: ViewModifier {
    let text: Text?

    func body(content: Content) -> some View {
        if let text {
            content.navigationSubtitle(text)
        } else {
            content
        }
    }
}
