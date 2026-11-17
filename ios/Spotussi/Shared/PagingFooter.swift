import SwiftUI

/// The end of a list that pages: a spinner that asks for the next page as
/// it comes into view, or one row to ask again when the last page failed.
struct PagingFooter: View {
    let isFailed: Bool
    let loadMore: () -> Void
    let retry: () -> Void

    var body: some View {
        if isFailed {
            Button(action: retry) {
                HStack(spacing: 8) {
                    Text("Couldn’t load more.")
                        .foregroundStyle(.secondary)
                    Spacer(minLength: 8)
                    Label("Retry", systemImage: "arrow.clockwise")
                        .fontWeight(.medium)
                }
                .font(.subheadline)
                .frame(maxWidth: .infinity, minHeight: 44)
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, minHeight: 44)
                // Reaching the end without a row near it having asked (a
                // short page, a fast fling) asks here.
                .onAppear(perform: loadMore)
                .accessibilityLabel("Loading more")
        }
    }
}
