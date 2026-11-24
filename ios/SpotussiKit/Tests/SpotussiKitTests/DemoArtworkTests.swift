import CoreGraphics
import Foundation
import Testing

@testable import SpotussiKit

@Suite struct DemoArtworkTests {
    @Test func drawsAtTheSizeAsked() throws {
        let image = try #require(DemoArtwork.draw("/_mock/img/album/abc.svg", side: 120))
        #expect(image.width == 120)
        #expect(image.height == 120)
        #expect(DemoArtwork.draw("/_mock/img/album/abc.svg", side: 0) == nil)
    }

    @Test func sameAddressSameTile() throws {
        let a = try #require(DemoArtwork.draw("/_mock/img/album/abc.svg", side: 64))
        let b = try #require(DemoArtwork.draw("/_mock/img/album/abc.svg", side: 64))
        let c = try #require(DemoArtwork.draw("/_mock/img/album/xyz.svg", side: 64))
        #expect(pixels(a) == pixels(b))
        #expect(pixels(a) != pixels(c))
    }

    @Test(arguments: [
        ("/_mock/img/artist/a.svg", DemoArtwork.Kind.artist),
        ("/_mock/img/album/a.svg", .album),
        ("/_mock/img/playlist/a.svg", .playlist),
        ("https://i.scdn.co/image/a", .other),
        ("", .other),
    ])
    func kinds(_ url: String, _ kind: DemoArtwork.Kind) {
        #expect(DemoArtwork.kind(of: url) == kind)
    }

    /// Every address in the bundled demo draws.
    @Test func everyFixtureImageDraws() throws {
        let pattern = try Regex(#""image_url":"([^"]+)""#)
        var urls = Set<String>()
        for operation in Fixtures.operations {
            let text = String(decoding: try #require(Fixtures.data(for: operation)), as: UTF8.self)
            for match in text.matches(of: pattern) {
                if let url = match.output[1].substring { urls.insert(String(url)) }
            }
        }
        #expect(urls.count > 100)
        for url in urls { #expect(DemoArtwork.draw(url, side: 16) != nil) }
    }

    private func pixels(_ image: CGImage) -> Data? {
        image.dataProvider?.data as Data?
    }
}
