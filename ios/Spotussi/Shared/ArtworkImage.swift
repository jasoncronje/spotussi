import CoreGraphics
import SpotussiKit
import SwiftUI

/// Artwork from the API at a small size: a recessed tile until the image
/// arrives, and for good when there is none or it can't be drawn. Loads
/// through the app's artwork cache, decodes off the main actor at the size
/// shown, and stops loading when the row goes away.
struct ArtworkImage: View {
    let url: String?
    /// Shown on the empty tile.
    let symbol: String
    var isRound = false
    @ScaledMetric private var side: CGFloat

    @Environment(Session.self) private var session
    @Environment(\.displayScale) private var displayScale
    @State private var image: CGImage?

    init(url: String?, side: CGFloat, symbol: String, isRound: Bool = false) {
        self.url = url
        self.symbol = symbol
        self.isRound = isRound
        _side = ScaledMetric(wrappedValue: side, relativeTo: .body)
    }

    var body: some View {
        tile
            .frame(width: side, height: side)
            .clipShape(isRound ? AnyShape(.circle) : AnyShape(.rect(cornerRadius: side * 0.12)))
            .task(id: Load(url: url, pixels: pixels)) { await load() }
            .accessibilityHidden(true)
    }

    @ViewBuilder private var tile: some View {
        if let image {
            Image(decorative: image, scale: displayScale)
                .resizable()
                .scaledToFill()
        } else {
            Rectangle()
                .fill(.ground)
                .overlay {
                    Image(systemName: symbol)
                        .font(.system(size: side * 0.4))
                        .foregroundStyle(.grille)
                }
        }
    }

    /// What decides the image: the address and the size it is decoded at.
    private nonisolated struct Load: Hashable {
        let url: String?
        let pixels: Int
    }

    private var pixels: Int { Int((side * displayScale).rounded(.up)) }

    private func load() async {
        guard let url, let request = session.artworkRequest(for: url) else {
            image = nil
            return
        }
        let key = "\(request.url?.absoluteString ?? url)#\(pixels)"
        if let cached = ArtworkMemory.shared.image(for: key) {
            image = cached
            return
        }
        // Another address: the old image would be the wrong one meanwhile.
        image = nil
        guard let loaded = try? await Artwork.image(for: request, maxPixelSize: pixels),
            !Task.isCancelled
        else { return }
        ArtworkMemory.shared.store(loaded, for: key)
        image = loaded
    }
}

/// Decoded artwork kept in memory, so scrolling back and switching tabs
/// don't decode again. The system empties it under memory pressure.
private final class ArtworkMemory {
    static let shared = ArtworkMemory()
    private let cache = NSCache<NSString, CGImage>()

    private init() {
        cache.countLimit = 300
    }

    func image(for key: String) -> CGImage? { cache.object(forKey: key as NSString) }
    func store(_ image: CGImage, for key: String) { cache.setObject(image, forKey: key as NSString) }
}
