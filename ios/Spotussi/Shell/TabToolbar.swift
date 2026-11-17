import SpotussiKit
import SwiftUI

extension View {
    /// Range on the leading side; Plays/Time and Settings on the trailing
    /// side, as separate glass groups.
    func tabToolbar(_ controls: TabControls) -> some View {
        modifier(TabToolbar(controls: controls))
    }
}

private struct TabToolbar: ViewModifier {
    let controls: TabControls
    @State private var isShowingSettings = false
    @State private var isSearching = false

    func body(content: Content) -> some View {
        content
            .toolbar {
                if controls.range {
                    ToolbarItem(placement: .topBarLeading) {
                        RangeMenu()
                    }
                }
                if let screen = controls.measure {
                    // Centred in the bar, between the range and the buttons.
                    ToolbarItem(placement: .principal) {
                        MeasurePicker(screen: screen)
                    }
                    // The segmented control draws its own glass; the toolbar's
                    // pill around it showed as a second outline.
                    .sharedBackgroundVisibility(.hidden)
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Button {
                        isSearching = true
                    } label: {
                        Label("Search", systemImage: "magnifyingglass")
                    }
                    Button {
                        isShowingSettings = true
                    } label: {
                        Label("Settings", systemImage: "gearshape")
                    }
                }
            }
            .sheet(isPresented: $isSearching) {
                SearchSheet()
            }
            .sheet(isPresented: $isShowingSettings) {
                SettingsSheet()
            }
    }
}
