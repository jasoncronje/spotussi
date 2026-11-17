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

    func body(content: Content) -> some View {
        content
            .toolbar {
                if controls.range {
                    ToolbarItem(placement: .topBarLeading) {
                        RangeMenu()
                    }
                }
                if let screen = controls.measure {
                    ToolbarItem(placement: .topBarTrailing) {
                        MeasurePicker(screen: screen)
                    }
                    ToolbarSpacer(.fixed, placement: .topBarTrailing)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isShowingSettings = true
                    } label: {
                        Label("Settings", systemImage: "gearshape")
                    }
                }
            }
            .sheet(isPresented: $isShowingSettings) {
                SettingsSheet()
            }
    }
}
