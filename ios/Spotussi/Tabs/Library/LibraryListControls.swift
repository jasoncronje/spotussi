import SpotussiAPI
import SpotussiKit
import SwiftUI

/// Songs, Albums, Playlists: segmented while the labels fit, a menu at the
/// largest text sizes.
struct LibraryPartPicker: View {
    @Binding var selection: LibraryModel.Part

    var body: some View {
        ViewThatFits(in: .horizontal) {
            picker.pickerStyle(.segmented)
            picker.pickerStyle(.menu)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var picker: some View {
        Picker("Show", selection: $selection) {
            Text("Songs").tag(LibraryModel.Part.songs)
            Text("Albums").tag(LibraryModel.Part.albums)
            Text("Playlists").tag(LibraryModel.Part.playlists)
        }
    }
}

/// Above the list on screen: how many it holds, and one menu with the
/// sort and filters the API offers for it.
struct LibraryListBar: View {
    @Bindable var model: LibraryModel

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .center) {
                count
                Spacer(minLength: 12)
                menu
            }
            VStack(alignment: .leading, spacing: 4) {
                count
                menu
            }
        }
    }

    // MARK: - Count

    /// "210 never played", "158 albums", "12 yours", once the list answers
    /// what is chosen.
    @ViewBuilder private var count: some View {
        switch model.part {
        case .songs:
            if let query = model.songsQuery, let total = total(model.songs, query) {
                CountLine(text: songCount(total, query.filter))
            }
        case .albums:
            if let total = total(model.albums, model.albumSort) {
                CountLine(text: Text("\(total) albums"))
            }
        case .playlists:
            if let total = total(model.playlists, model.playlistQuery) {
                CountLine(text: playlistCount(total, model.playlistQuery.relation))
            }
        }
    }

    private func total<Item, Query>(_ list: PagedListModel<Item, Query>, _ query: Query) -> Int? {
        guard list.answered == query else { return nil }
        return list.list?.total
    }

    private func songCount(_ total: Int, _ filter: SavedSong.Filter) -> Text {
        switch filter {
        case .all: Text("\(total) songs")
        case .neverPlayed: Text("\(total) never played")
        case .maybePlayed: Text("\(total) other release")
        case .played: Text("\(total) played")
        }
    }

    private func playlistCount(_ total: Int, _ relation: SavedPlaylist.RelationFilter) -> Text {
        switch relation {
        case .all: Text("\(total) playlists")
        case .own: Text("\(total) yours")
        case .collaborative: Text("\(total) collaborative")
        case .followed: Text("\(total) followed")
        }
    }

    // MARK: - Menu

    private var menu: some View {
        Menu {
            switch model.part {
            case .songs: songOptions
            case .albums: albumOptions
            case .playlists: playlistOptions
            }
        } label: {
            Label(sortName, systemImage: "line.3.horizontal.decrease")
                .font(.subheadline.weight(.medium))
                .frame(minHeight: 44)
                .contentShape(.rect)
        }
        .tint(.primary)
        .accessibilityLabel(String(localized: "Sort and filter"))
        .accessibilityValue(sortName)
    }

    @ViewBuilder private var songOptions: some View {
        let counts = model.summary?.tracks
        Section("Show") {
            Picker("Show", selection: songFilter) {
                option("Never played", counts?.neverPlayed).tag(SavedSong.Filter.neverPlayed)
                option("Other release", counts?.maybePlayed).tag(SavedSong.Filter.maybePlayed)
                option("Played", counts?.played).tag(SavedSong.Filter.played)
                option("All", model.summary?.savedTracks).tag(SavedSong.Filter.all)
            }
            .pickerStyle(.inline)
        }
        Section("Sort") {
            Picker("Sort", selection: $model.songSort) {
                Text("Recently saved").tag(SavedSong.Sort.added)
                Text("Most played").tag(SavedSong.Sort.plays)
                Text("Last played").tag(SavedSong.Sort.lastPlayed)
            }
            .pickerStyle(.inline)
        }
    }

    @ViewBuilder private var albumOptions: some View {
        Section("Sort") {
            Picker("Sort", selection: $model.albumSort) {
                Text("Recently saved").tag(SavedAlbum.Sort.added)
                Text("Most played").tag(SavedAlbum.Sort.plays)
                Text("Most heard").tag(SavedAlbum.Sort.playedShare)
            }
            .pickerStyle(.inline)
        }
    }

    @ViewBuilder private var playlistOptions: some View {
        Section("Show") {
            Picker("Show", selection: $model.playlistQuery.relation) {
                Text("All").tag(SavedPlaylist.RelationFilter.all)
                Text("Yours").tag(SavedPlaylist.RelationFilter.own)
                Text("Collaborative").tag(SavedPlaylist.RelationFilter.collaborative)
                Text("Followed").tag(SavedPlaylist.RelationFilter.followed)
            }
            .pickerStyle(.inline)
        }
        Section("Sort") {
            Picker("Sort", selection: $model.playlistQuery.sort) {
                Text("Most played").tag(SavedPlaylist.Sort.playsHere)
                Text("Most heard").tag(SavedPlaylist.Sort.playedShare)
                Text("Name").tag(SavedPlaylist.Sort.name)
            }
            .pickerStyle(.inline)
        }
    }

    /// "Never played · 210"
    private func option(_ name: LocalizedStringKey, _ count: Int?) -> Text {
        guard let count else { return Text(name) }
        return Text("\(Text(name)) · \(Figures.count(count))")
    }

    /// The chosen filter, or the web's default until one is chosen.
    private var songFilter: Binding<SavedSong.Filter> {
        Binding(
            get: { model.songsQuery?.filter ?? .neverPlayed },
            set: { model.songFilter = $0 })
    }

    private var sortName: String {
        switch model.part {
        case .songs:
            switch model.songSort {
            case .added: String(localized: "Recently saved")
            case .plays: String(localized: "Most played")
            case .lastPlayed: String(localized: "Last played")
            }
        case .albums:
            switch model.albumSort {
            case .added: String(localized: "Recently saved")
            case .plays: String(localized: "Most played")
            case .playedShare: String(localized: "Most heard")
            }
        case .playlists:
            switch model.playlistQuery.sort {
            case .playsHere: String(localized: "Most played")
            case .playedShare: String(localized: "Most heard")
            case .name: String(localized: "Name")
            }
        }
    }
}
