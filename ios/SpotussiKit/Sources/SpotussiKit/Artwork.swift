import CoreGraphics
import Foundation
import ImageIO

extension URLSession {
    /// Artwork, with a disk cache of its own so images never push API
    /// answers out of theirs. Artwork addresses don't change, so the
    /// system's HTTP caching keeps them across launches.
    public static let artwork: URLSession = {
        let config = URLSessionConfiguration.default
        config.urlCache = URLCache(
            memoryCapacity: 4 << 20, diskCapacity: 128 << 20,
            directory: URL.cachesDirectory.appending(path: "artwork"))
        config.requestCachePolicy = .useProtocolCachePolicy
        config.timeoutIntervalForRequest = 20
        return URLSession(configuration: config)
    }()
}

/// Loads artwork and decodes it at the size it is shown, off the main actor.
public enum Artwork {
    /// The image at `request`, no more than `maxPixelSize` on its longer
    /// side; nil when there is nothing ImageIO can draw (the mock server's
    /// SVGs, an error page). Cancelling the calling task cancels the load.
    @concurrent
    public static func image(
        for request: URLRequest, maxPixelSize: Int, session: URLSession = .artwork
    ) async throws -> CGImage? {
        let (data, response) = try await session.data(for: request, delegate: RedirectGuard())
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
            http.mimeType != "image/svg+xml"
        else { return nil }
        try Task.checkCancellation()
        return downsample(data, maxPixelSize: maxPixelSize)
    }

    /// Decodes straight to a thumbnail, so a 640-pixel cover shown at 40
    /// points never sits in memory at full size.
    static func downsample(_ data: Data, maxPixelSize: Int) -> CGImage? {
        let sourceOptions = [kCGImageSourceShouldCache: false] as CFDictionary
        guard let source = CGImageSourceCreateWithData(data as CFData, sourceOptions) else { return nil }
        let options =
            [
                kCGImageSourceCreateThumbnailFromImageAlways: true,
                kCGImageSourceShouldCacheImmediately: true,
                kCGImageSourceCreateThumbnailWithTransform: true,
                kCGImageSourceThumbnailMaxPixelSize: max(1, maxPixelSize),
            ] as CFDictionary
        return CGImageSourceCreateThumbnailAtIndex(source, 0, options)
    }
}

/// Keeps the token on the instance: a redirect to another host (a CDN, an
/// image proxy) goes without the `Authorization` header.
final class RedirectGuard: NSObject, URLSessionTaskDelegate, Sendable {
    func urlSession(
        _ session: URLSession, task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest
    ) async -> URLRequest? {
        guard let origin = task.originalRequest?.url else { return request }
        return Self.forwarded(request, from: origin)
    }

    static func forwarded(_ request: URLRequest, from origin: URL) -> URLRequest {
        guard let target = request.url, !target.hasSameOrigin(as: origin) else { return request }
        var request = request
        request.setValue(nil, forHTTPHeaderField: "Authorization")
        return request
    }
}

extension URL {
    /// Same scheme, host and port, a missing port counting as the
    /// scheme's default.
    func hasSameOrigin(as other: URL) -> Bool {
        guard let scheme = scheme?.lowercased(), scheme == other.scheme?.lowercased(),
            let host = host()?.lowercased(), host == other.host()?.lowercased()
        else { return false }
        func port(_ url: URL) -> Int? { url.port ?? (scheme == "https" ? 443 : scheme == "http" ? 80 : nil) }
        return port(self) == port(other)
    }
}
