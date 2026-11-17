import SwiftUI

/// Why pairing failed and what to do next. VoiceOver hears it as soon as
/// it appears.
struct ProblemMessage: View {
    let problem: PairingProblem

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text(problem.title)
                    .font(.subheadline.weight(.semibold))
                Text(problem.advice)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
        .onChange(of: problem, initial: true) {
            AccessibilityNotification.Announcement(problem.title + " " + problem.advice).post()
        }
    }
}
