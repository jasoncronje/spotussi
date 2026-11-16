import Foundation
import Testing

@testable import SpotussiKit

@Suite struct PairingLinkTests {
    @Test func readsTheLinkTheWebAppMakes() throws {
        // What `new URLSearchParams({url, token})` produces in web/src/lib/pairing.ts.
        let url = URL(
            string: "spotussi://pair?url=https%3A%2F%2Fstats.example.com&token=spt_abcDEF123-_x")!
        let link = try PairingLink(url)
        #expect(link.instance.baseURL.absoluteString == "https://stats.example.com")
        #expect(link.instance.apiURL.absoluteString == "https://stats.example.com/api/v1")
        #expect(link.token == "spt_abcDEF123-_x")
    }

    @Test func keepsThePortAndPlainHTTP() throws {
        let url = URL(string: "spotussi://pair?url=http%3A%2F%2F10.0.0.5%3A8080&token=spt_abcdef12")!
        let link = try PairingLink(url)
        #expect(link.instance.baseURL.absoluteString == "http://10.0.0.5:8080")
        #expect(link.instance.displayName == "10.0.0.5:8080")
        #expect(link.instance.isPlainHTTP)
    }

    @Test(arguments: [
        "https://stats.example.com/settings/tokens",
        "spotussi://open?url=https%3A%2F%2Fa.example&token=spt_abcdef12",
        "other://pair?url=https%3A%2F%2Fa.example&token=spt_abcdef12",
    ])
    func refusesOtherLinks(_ text: String) {
        #expect(throws: PairingLink.Failure.notAPairingLink) { try PairingLink(URL(string: text)!) }
    }

    @Test(arguments: [
        "spotussi://pair?token=spt_abcdef12",
        "spotussi://pair?url=ftp%3A%2F%2Fa.example&token=spt_abcdef12",
        "spotussi://pair?url=&token=spt_abcdef12",
    ])
    func refusesABadAddress(_ text: String) {
        #expect(throws: PairingLink.Failure.badAddress) { try PairingLink(URL(string: text)!) }
    }

    @Test(arguments: [
        "spotussi://pair?url=https%3A%2F%2Fa.example",
        "spotussi://pair?url=https%3A%2F%2Fa.example&token=abcdef123456",
        "spotussi://pair?url=https%3A%2F%2Fa.example&token=spt_",
        "spotussi://pair?url=https%3A%2F%2Fa.example&token=spt_abc%20def12",
    ])
    func refusesABadToken(_ text: String) {
        #expect(throws: PairingLink.Failure.badToken) { try PairingLink(URL(string: text)!) }
    }

    @Test(arguments: [
        ("stats.example.com", "https://stats.example.com"),
        ("  https://stats.example.com/  ", "https://stats.example.com"),
        ("https://stats.example.com/settings/tokens?x=1#y", "https://stats.example.com"),
        ("HTTP://192.168.1.20:8080/", "http://192.168.1.20:8080"),
        ("https://user:secret@stats.example.com", "https://stats.example.com"),
    ])
    func tidiesATypedAddress(_ typed: String, _ expected: String) throws {
        let link = try PairingLink(address: typed, token: " spt_abcdef12\n")
        #expect(link.instance.baseURL.absoluteString == expected)
        #expect(link.token == "spt_abcdef12")
    }

    @Test func refusesAnEmptyAddress() {
        #expect(throws: PairingLink.Failure.badAddress) {
            try PairingLink(address: "   ", token: "spt_abcdef12")
        }
    }
}
