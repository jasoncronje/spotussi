import Foundation

/// One page of a list the API pages by offset, as it answered.
public struct OffsetPage<Item: Sendable>: Sendable {
    public let items: [Item]
    /// Where the server says this page starts.
    public let offset: Int
    /// Items in the whole list.
    public let total: Int

    public init(items: [Item], offset: Int, total: Int) {
        self.items = items
        self.offset = offset
        self.total = total
    }
}

/// A list as far as it has been paged by offset, with no cap on its length.
/// Complete when the server has nothing more to give, by the same rule as
/// ``RankedList``.
public struct OffsetList<Item: Identifiable & Hashable & Sendable>: Hashable, Sendable
where Item.ID: Sendable {
    public private(set) var items: [Item] = []
    public private(set) var total: Int
    public private(set) var isComplete = false
    /// Where the next page starts.
    public private(set) var nextOffset = 0
    private var seen: Set<Item.ID> = []

    public init(first page: OffsetPage<Item>) {
        total = page.total
        append(page, requestedAt: 0)
    }

    public var isEmpty: Bool { items.isEmpty }

    /// Adds the page asked for at `offset`. Items already listed are
    /// dropped, in case the list moved between two pages.
    public mutating func append(_ page: OffsetPage<Item>, requestedAt offset: Int) {
        let fresh = page.items.filter { seen.insert($0.id).inserted }
        items += fresh
        total = page.total
        nextOffset = page.offset + page.items.count
        // A server that ignores the offset answers with the first page again
        // (the demo data does): stop there rather than repeat it for ever.
        isComplete = fresh.isEmpty || page.offset != offset || nextOffset >= page.total
    }
}
