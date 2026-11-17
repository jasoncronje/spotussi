import Foundation
import SpotussiAPI
import Testing

@testable import SpotussiKit

@Suite struct DeepLinkTests {
    @Test(arguments: [
        ("spotussi://artist/4Z8W4fKeB5YxbusRsdQVPb", EntityKind.artist, "4Z8W4fKeB5YxbusRsdQVPb"),
        ("spotussi://album/abc", .album, "abc"),
        ("spotussi://track/t1", .track, "t1"),
        ("SPOTUSSI://Genre/modern%20pop", .genre, "modern pop"),
    ])
    func entityLinks(_ address: String, _ kind: EntityKind, _ id: String) throws {
        let link = try #require(DeepLink(URL(string: address)!))
        #expect(link == .entity(EntityLink(kind: kind, id: id, name: "")))
    }

    @Test func aNameTitlesThePage() throws {
        let link = try #require(DeepLink(URL(string: "spotussi://artist/a1?name=Iris%20Sakoti")!))
        #expect(link == .entity(EntityLink(kind: .artist, id: "a1", name: "Iris Sakoti")))
    }

    @Test func playlistLinks() throws {
        let link = try #require(DeepLink(URL(string: "spotussi://playlist/37i9dQ")!))
        #expect(link == .playlist(id: "37i9dQ", name: ""))
    }

    @Test(arguments: [
        "spotussi://pair?url=https://stats.example.com&token=spt_abcdef12",
        "spotussi://artist",
        "spotussi://artist/",
        "spotussi://artist/a/b",
        "spotussi://session/1",
        "https://artist/a1",
    ])
    func otherLinksAreNotDeepLinks(_ address: String) {
        #expect(DeepLink(URL(string: address)!) == nil)
    }

    @Test func playlistContextsKnowTheirLibraryID() async throws {
        let contexts = PlayContexts(try await Client.demo.getContexts().ok.body.json)
        let playlists = contexts.items.filter { $0.kind == .playlist }
        #expect(!playlists.isEmpty)
        for playlist in playlists {
            #expect(playlist.playlistID.map { "spotify:playlist:\($0)" } == playlist.id)
        }
        #expect(contexts.items.filter { $0.kind != .playlist }.allSatisfy { $0.playlistID == nil })
    }
}
