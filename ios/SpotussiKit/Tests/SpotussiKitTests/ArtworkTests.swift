import CoreGraphics
import Foundation
import ImageIO
import Testing
import UniformTypeIdentifiers

@testable import SpotussiKit

@Suite struct ArtworkTests {
    @Test func decodesAtTheSizeShown() throws {
        let image = try #require(Artwork.downsample(try png(width: 400, height: 200), maxPixelSize: 40))
        #expect(image.width == 40)
        #expect(image.height == 20)
    }

    @Test func svgIsNotDrawn() {
        let svg = Data(#"<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"/>"#.utf8)
        #expect(Artwork.downsample(svg, maxPixelSize: 40) == nil)
    }

    @Test func redirectsElsewhereDropTheToken() throws {
        let origin = try #require(URL(string: "https://stats.example.com/img/a.jpg"))
        var request = URLRequest(url: try #require(URL(string: "https://i.scdn.co/image/a")))
        request.setValue("Bearer spt_abcdef12", forHTTPHeaderField: "Authorization")
        #expect(RedirectGuard.forwarded(request, from: origin).value(forHTTPHeaderField: "Authorization") == nil)

        request.url = URL(string: "https://stats.example.com/img/b.jpg")
        #expect(
            RedirectGuard.forwarded(request, from: origin).value(forHTTPHeaderField: "Authorization")
                == "Bearer spt_abcdef12")
    }

    @Test(arguments: [
        ("https://stats.example.com", "https://STATS.example.com:443/x", true),
        ("http://10.0.0.5:8080", "http://10.0.0.5:8080/img", true),
        ("https://stats.example.com", "http://stats.example.com", false),
        ("https://stats.example.com", "https://stats.example.com:8443", false),
        ("https://stats.example.com", "https://images.example.com", false),
    ])
    func origins(_ a: String, _ b: String, _ same: Bool) throws {
        let first = try #require(URL(string: a))
        let second = try #require(URL(string: b))
        #expect(first.hasSameOrigin(as: second) == same)
    }

    @Test func pollingBacksOffAndRecovers() {
        var schedule = PollSchedule()
        #expect(schedule.delay == .seconds(15))
        var delays: [Duration] = []
        for _ in 0..<5 {
            schedule.failed()
            delays.append(schedule.delay)
        }
        #expect(delays == [.seconds(30), .seconds(60), .seconds(120), .seconds(120), .seconds(120)])
        #expect(schedule.lastAnswerIsStale)
        schedule.succeeded()
        #expect(schedule.delay == .seconds(15))
        schedule.failed()
        #expect(!schedule.lastAnswerIsStale)
    }

    private func png(width: Int, height: Int) throws -> Data {
        let context = try #require(
            CGContext(
                data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0,
                space: CGColorSpaceCreateDeviceRGB(),
                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        context.setFillColor(red: 0.1, green: 0.7, blue: 0.3, alpha: 1)
        context.fill(CGRect(x: 0, y: 0, width: width, height: height))
        let image = try #require(context.makeImage())
        let data = NSMutableData()
        let destination = try #require(
            CGImageDestinationCreateWithData(data, UTType.png.identifier as CFString, 1, nil))
        CGImageDestinationAddImage(destination, image, nil)
        #expect(CGImageDestinationFinalize(destination))
        return data as Data
    }
}
