import SpotussiAPI
import SpotussiKit

/// Rows drawn redacted while a list's first page loads: names of a usual
/// length, so the placeholder is the height of the real thing.
extension SavedSong {
    static let placeholder = SavedSong(
        Components.Schemas.LibraryTrack(
            addedAt: 0, msPlayed: 0, plays: 2, status: .played,
            track: .init(artists: [.init(id: "", name: "Placeholder artist")], id: "placeholder", name: "Placeholder song")
        ))
}

extension SavedAlbum {
    static let placeholder = SavedAlbum(
        Components.Schemas.LibraryAlbum(
            addedAt: 0,
            album: .init(artists: [.init(id: "", name: "Placeholder artist")], id: "placeholder", name: "Placeholder album"),
            msPlayed: 0, plays: 0, tracksPlayed: 3, tracksTotal: 10))
}

extension SavedPlaylist {
    static let placeholder = SavedPlaylist(
        Components.Schemas.LibraryPlaylist(
            itemsAvailable: true, itemsTotal: 40, msHere: 0,
            playlist: .init(id: "placeholder", name: "Placeholder playlist", relation: .own, spotifyOwned: false, uri: ""),
            playsHere: 0, tracks: 40, tracksPlayed: 20))
}
