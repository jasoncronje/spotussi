import Foundation
import Security
import Synchronization

/// Where the paired instance's API token is kept. One token at a time: the
/// app pairs with one instance (D41).
public protocol TokenStore: Sendable {
    func token() throws -> String?
    func save(_ token: String) throws
    func delete() throws
}

/// The token as a generic password in the Keychain, readable after the first
/// unlock (so a background refresh can use it), on this device only, and
/// never synced to iCloud: a lost phone is cut off by revoking its token.
public struct KeychainTokenStore: TokenStore {
    public static let service = "app.spotussi.ios"
    private let account: String

    public init(account: String = "api-token") {
        self.account = account
    }

    public struct Failure: Error, Equatable, Sendable {
        public let status: OSStatus
    }

    public func token() throws -> String? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw Failure(status: status) }
        guard let data = result as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    public func save(_ token: String) throws {
        let attributes: [String: Any] = [
            kSecValueData as String: Data(token.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let status = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
        if status == errSecSuccess { return }
        guard status == errSecItemNotFound else { throw Failure(status: status) }
        let added = SecItemAdd(baseQuery.merging(attributes) { $1 } as CFDictionary, nil)
        guard added == errSecSuccess else { throw Failure(status: added) }
    }

    public func delete() throws {
        let status = SecItemDelete(baseQuery as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw Failure(status: status)
        }
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: account,
            kSecAttrSynchronizable as String: false,
        ]
    }
}

/// A token held in memory: tests, previews and demo builds.
public final class InMemoryTokenStore: TokenStore {
    private let stored: Mutex<String?>

    public init(token: String? = nil) {
        stored = Mutex(token)
    }

    public func token() -> String? { stored.withLock { $0 } }
    public func save(_ token: String) { stored.withLock { $0 = token } }
    public func delete() { stored.withLock { $0 = nil } }
}
