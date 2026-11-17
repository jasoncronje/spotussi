import CoreGraphics
import SpotussiKit
import SwiftUI

/// Artwork from the API at a small size: a recessed tile until the image
/// arrives, and for good when there is none or it can't be drawn.
struct ArtworkImage: View {
    let url: String?
    /// Shown on the empty tile.
    let symbol: String
    var isRound = false
    @ScaledMetric private var side: CGFloat
    @Environment(\.displayScale) private var displayScale

    init(url: String?, side: CGFloat, symbol: String, isRound: Bool = false) {
        self.url = url
        self.symbol = symbol
        self.isRound = isRound
        _side = ScaledMetric(wrappedValue: side, relativeTo: .body)
    }

    var body: some View {
        ArtworkLoader(url: url, pixels: Int((side * displayScale).rounded(.up))) {
            Rectangle()
                .fill(.ground)
                .overlay {
                    Image(systemName: symbol)
                        .font(.system(size: side * 0.4))
                        .foregroundStyle(.grille)
                }
        }
        .frame(width: side, height: side)
        .clipShape(isRound ? AnyShape(.circle) : AnyShape(.rect(cornerRadius: side * 0.12)))
        .accessibilityHidden(true)
    }
}

/// Artwork filling whatever space it is given, for the head of an entity
/// page. Decoded at the size it is drawn; until then, and when there is
/// none, a graphite field with the kind's symbol.
struct HeroArtwork: View {
    let url: String?
    let symbol: String
    @State private var pixels = 0
    @Environment(\.displayScale) private var displayScale

    var body: some View {
        ArtworkLoader(url: url, pixels: pixels) {
            LinearGradient(
                colors: [Color(hex: 0x2A2D35), Color(hex: 0x121317)], startPoint: .top, endPoint: .bottom
            )
            .overlay(alignment: .topTrailing) {
                Image(systemName: symbol)
                    .font(.system(size: 120, weight: .light))
                    .foregroundStyle(Color(hex: 0x3A3D46))
                    .padding(.top, 36)
                    .padding(.trailing, 24)
            }
        }
        .onGeometryChange(for: CGFloat.self) { geometry in
            max(geometry.size.width, geometry.size.height)
        } action: { side in
            pixels = Int((side * displayScale).rounded(.up))
        }
        .accessibilityHidden(true)
    }
}

/// Loads one image through the app's artwork cache, decodes it off the main
/// actor at `pixels` on its longer side, and stops loading when the view
/// goes away. Shows `placeholder` meanwhile.
private struct ArtworkLoader<Placeholder: View>: View {
    let url: String?
    let pixels: Int
    @ViewBuilder let placeholder: Placeholder

    @Environment(Session.self) private var session
    @Environment(\.displayScale) private var displayScale
    @State private var image: CGImage?
    /// The address `image` was loaded from.
    @State private var imageURL: String?

    var body: some View {
        Group {
            if let image {
                Image(decorative: image, scale: displayScale)
                    .resizable()
                    .scaledToFill()
            } else {
                placeholder
            }
        }
        .task(id: Load(url: url, pixels: pixels)) { await load() }
    }

    /// What decides the image: the address and the size it is decoded at.
    private nonisolated struct Load: Hashable {
        let url: String?
        let pixels: Int
    }

    private func load() async {
        guard let url, let request = session.artworkRequest(for: url) else {
            show(nil, from: nil)
            return
        }
        // Not measured yet: nothing to decode for.
        guard pixels > 0 else { return }
        let key = "\(request.url?.absoluteString ?? url)#\(pixels)"
        if let cached = ArtworkMemory.shared.image(for: key) {
            show(cached, from: url)
            return
        }
        // Another address: the old image would be the wrong one meanwhile.
        // A new size of the same one stays, scaled, until the sharper one is in.
        if imageURL != url { show(nil, from: nil) }
        guard let loaded = try? await Artwork.image(for: request, maxPixelSize: pixels),
            !Task.isCancelled
        else { return }
        ArtworkMemory.shared.store(loaded, for: key)
        show(loaded, from: url)
    }

    private func show(_ newImage: CGImage?, from url: String?) {
        image = newImage
        imageURL = url
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
