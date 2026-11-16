import SpotussiAPI
import SpotussiKit
import SwiftUI

/// The first screen until pairing and the tab shell land (I1): the brand,
/// and the last four weeks from the bundled sample data. It proves the path
/// every later screen uses: generated client, decoded types, brand tokens.
struct DemoView: View {
    @State private var model = DemoModel()

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                header
                switch model.state {
                case .loading:
                    ProgressView().frame(maxWidth: .infinity, minHeight: 200)
                case .failed(let message):
                    ContentUnavailableView(
                        "Sample data didn't load", systemImage: "exclamationmark.triangle",
                        description: Text(message))
                case .loaded(let summary):
                    readouts(summary)
                }
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(.ground)
        .task { await model.load() }
    }

    private var header: some View {
        HStack(spacing: 12) {
            LevelMark().frame(width: 36, height: 36)
            Text(verbatim: "spotussi")
                .font(.system(.title2, weight: .bold).width(.expanded))
            Spacer()
            Text("Demo")
                .font(.readoutLabel)
                .foregroundStyle(.lampInk)
                .padding(.horizontal, 10)
                .padding(.vertical, 5)
                .background(.panel, in: .capsule)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Spotussi, demo")
        .accessibilityAddTraits(.isHeader)
    }

    private func readouts(_ summary: DemoModel.Summary) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Last 4 Weeks")
                .font(.readoutLabel)
                .foregroundStyle(.secondary)
            Readout(label: "Time Listened", value: summary.listened)
            Readout(label: "Plays", value: summary.plays.formatted())
            Readout(label: "Different Artists", value: summary.artists.formatted())
        }
    }
}

private struct Readout: View {
    let label: LocalizedStringKey
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label).font(.readoutLabel).foregroundStyle(.secondary)
            Text(value).font(.readout()).contentTransition(.numericText())
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.panel, in: .rect(cornerRadius: 20))
        .accessibilityElement(children: .combine)
    }
}

@Observable
final class DemoModel {
    struct Summary: Equatable {
        var listened: String
        var plays: Int
        var artists: Int
    }

    enum State: Equatable {
        case loading
        case loaded(Summary)
        case failed(String)
    }

    private(set) var state: State = .loading
    private let client: Client

    init(client: Client = .demo) {
        self.client = client
    }

    func load() async {
        do {
            let current = try await client.getOverviewSummary().ok.body.json.current
            let listened = Duration.milliseconds(current.msPlayed)
                .formatted(.units(allowed: [.hours, .minutes], width: .abbreviated))
            state = .loaded(
                Summary(listened: listened, plays: current.plays, artists: current.artists))
        } catch {
            state = .failed(error.localizedDescription)
        }
    }
}

#Preview {
    DemoView()
}
