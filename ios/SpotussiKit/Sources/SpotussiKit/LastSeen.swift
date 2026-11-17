import Foundation
import HTTPTypes
import OpenAPIRuntime
import Synchronization

/// Keeps the last answer to every read and serves it when the server can't
/// be reached, so screens show what was last seen instead of failing.
///
/// Each GET carries the stored answer's ETag, so an unchanged answer comes
/// back as a 304 with no body and the stored one is used. The fallback
/// applies to connection failures and timeouts only: an answer from the
/// server, a refusal (401, 403) or any other HTTP error, is passed on as it
/// came. Answers are keyed by their full address, which starts with the
/// instance's own, so one instance's data never shows for another.
public struct LastSeenTransport: ClientTransport {
    private let base: any ClientTransport
    private let cache: URLCache
    private let now: @Sendable () -> Date

    /// Live answers only: a stale now-playing would claim a song is still
    /// playing, and `/health` is how pairing tells a server is there.
    static let liveOperations: Set<String> = ["getHealth", "getNowPlaying"]

    /// Bodies larger than this are passed through without being kept.
    private static let maximumBody = 8 << 20
    private static let storedAtKey = "storedAt"

    public init(base: any ClientTransport, cache: URLCache, now: @escaping @Sendable () -> Date = Date.init) {
        self.base = base
        self.cache = cache
        self.now = now
    }

    public func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        guard request.method == .get, !Self.liveOperations.contains(operationID),
            let key = Self.key(for: request, baseURL: baseURL)
        else {
            return try await base.send(request, body: body, baseURL: baseURL, operationID: operationID)
        }
        let stored = cache.cachedResponse(for: key)
        var request = request
        if let etag = (stored?.response as? HTTPURLResponse)?.value(forHTTPHeaderField: "ETag") {
            request.headerFields[.ifNoneMatch] = etag
        }

        let response: HTTPResponse
        let responseBody: HTTPBody?
        do {
            (response, responseBody) = try await base.send(
                request, body: body, baseURL: baseURL, operationID: operationID)
        } catch {
            guard Self.isUnreachable(error), let stored else { throw error }
            LastSeen.observer?.record(Self.storedAt(stored))
            return Self.reply(from: stored)
        }

        switch response.status.code {
        case 304:
            guard let stored else { return (response, responseBody) }
            return Self.reply(from: stored)
        case 200..<300:
            let data = try await Data(collecting: responseBody ?? HTTPBody(), upTo: Self.maximumBody)
            store(data, response: response, for: key)
            return (response, HTTPBody(data))
        case 502, 503, 504:
            // A proxy in front of a stopped server answers for it, which is
            // the same thing to the owner as no answer at all.
            guard let stored else { return (response, responseBody) }
            LastSeen.observer?.record(Self.storedAt(stored))
            return Self.reply(from: stored)
        default:
            return (response, responseBody)
        }
    }

    // MARK: - Storage

    /// The request's full address: the instance's API address, the path and
    /// the query.
    static func key(for request: HTTPRequest, baseURL: URL) -> URLRequest? {
        guard let path = request.path else { return nil }
        let base = baseURL.absoluteString.hasSuffix("/") ? String(baseURL.absoluteString.dropLast()) : baseURL.absoluteString
        guard let url = URL(string: base + path) else { return nil }
        return URLRequest(url: url)
    }

    private func store(_ data: Data, response: HTTPResponse, for key: URLRequest) {
        guard let url = key.url else { return }
        // What decoding and revalidating need, and nothing else.
        var headers: [String: String] = [:]
        for name in [HTTPField.Name.contentType, .eTag] {
            if let value = response.headerFields[name] { headers[name.rawName] = value }
        }
        guard
            let stored = HTTPURLResponse(
                url: url, statusCode: response.status.code, httpVersion: "HTTP/1.1", headerFields: headers)
        else { return }
        cache.storeCachedResponse(
            CachedURLResponse(
                response: stored, data: data, userInfo: [Self.storedAtKey: now().timeIntervalSince1970],
                storagePolicy: .allowed),
            for: key)
    }

    private static func storedAt(_ stored: CachedURLResponse) -> Date? {
        (stored.userInfo?[storedAtKey] as? TimeInterval).map(Date.init(timeIntervalSince1970:))
    }

    private static func reply(from stored: CachedURLResponse) -> (HTTPResponse, HTTPBody?) {
        var fields = HTTPFields()
        var status = HTTPResponse.Status.ok
        if let http = stored.response as? HTTPURLResponse {
            status = .init(code: http.statusCode)
            for (name, value) in http.allHeaderFields {
                guard let name = name as? String, let value = value as? String,
                    let fieldName = HTTPField.Name(name)
                else { continue }
                fields[fieldName] = value
            }
        }
        return (HTTPResponse(status: status, headerFields: fields), HTTPBody(stored.data))
    }

    // MARK: - Failures

    /// No connection, or no answer in time. Everything else, including TLS
    /// failures and App Transport Security, is a real problem to show.
    static func isUnreachable(_ error: any Error) -> Bool {
        guard let urlError = error as? URLError else { return false }
        switch urlError.code {
        case .notConnectedToInternet, .networkConnectionLost, .cannotConnectToHost, .cannotFindHost,
            .dnsLookupFailed, .timedOut, .internationalRoamingOff, .dataNotAllowed, .callIsActive,
            .cannotLoadFromNetwork:
            return true
        default:
            return false
        }
    }
}

/// Tells a caller that answers it received came from the stored ones.
public enum LastSeen {
    /// Set around a request to learn whether it was served from storage.
    @TaskLocal public static var observer: Observer?

    public final class Observer: Sendable {
        private struct State {
            var served = false
            var oldest: Date?
        }

        private let state = Mutex(State())

        public init() {}

        /// Some answer came from storage.
        public var servedFromStorage: Bool { state.withLock { $0.served } }

        /// When the oldest answer served from storage was stored, if known.
        public var storedAt: Date? { state.withLock { $0.oldest } }

        func record(_ date: Date?) {
            state.withLock { state in
                state.served = true
                if let date { state.oldest = min(state.oldest ?? date, date) }
            }
        }
    }
}

extension URLCache {
    /// Answers from the API, for `LastSeenTransport`.
    public static let spotussiAPI = URLCache.protected(name: "api", memory: 8 << 20, disk: 64 << 20)

    /// A disk cache in its own directory under Caches, which the system
    /// never backs up, kept out of backups explicitly, and readable only
    /// once the phone has been unlocked after a restart: the answers came
    /// from requests that carried the token.
    static func protected(name: String, memory: Int, disk: Int) -> URLCache {
        let directory = URL.cachesDirectory.appending(path: name, directoryHint: .isDirectory)
        CacheDirectory.prepare(directory)
        return URLCache(memoryCapacity: memory, diskCapacity: disk, directory: directory)
    }
}

enum CacheDirectory {
    static func prepare(_ directory: URL) {
        let files = FileManager.default
        #if os(iOS)
            let protection: [FileAttributeKey: Any] = [
                .protectionKey: FileProtectionType.completeUntilFirstUserAuthentication
            ]
        #else
            let protection: [FileAttributeKey: Any] = [:]
        #endif
        try? files.createDirectory(at: directory, withIntermediateDirectories: true, attributes: protection)
        // A directory made by an earlier version keeps its old class.
        if !protection.isEmpty {
            try? files.setAttributes(protection, ofItemAtPath: directory.path(percentEncoded: false))
        }
        var directory = directory
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? directory.setResourceValues(values)
    }
}
