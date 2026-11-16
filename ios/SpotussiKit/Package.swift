// swift-tools-version: 6.2
import PackageDescription

// The iPhone app's code outside the app target (D41).
//
// SpotussiAPI is the client generated from api/openapi.yaml and checked in
// (ios/scripts/gen-api.sh), so a build needs no generator plugin.
// SpotussiKit holds what the screens share: pairing, the client set-up, the
// bundled demo data and the brand's colours and type.
//
// macOS is listed so `swift test` runs on the host without a simulator.
let package = Package(
    name: "SpotussiKit",
    platforms: [.iOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "SpotussiKit", targets: ["SpotussiKit"]),
        .library(name: "SpotussiAPI", targets: ["SpotussiAPI"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-runtime", from: "1.13.4"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", from: "1.4.0"),
    ],
    targets: [
        .target(
            name: "SpotussiAPI",
            dependencies: [
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
            ]
        ),
        .target(
            name: "SpotussiKit",
            dependencies: [
                "SpotussiAPI",
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
            ],
            resources: [.copy("Fixtures")]
        ),
        .testTarget(
            name: "SpotussiKitTests",
            dependencies: ["SpotussiKit", "SpotussiAPI"]
        ),
    ],
    swiftLanguageModes: [.v6]
)
