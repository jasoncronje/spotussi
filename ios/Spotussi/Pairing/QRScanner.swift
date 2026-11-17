import SwiftUI
import Vision
import VisionKit

/// VisionKit's live QR scanner: the app's one wrapped UIKit view. Reports
/// each new code it reads, once.
struct QRScanner: UIViewControllerRepresentable {
    let onCode: (String) -> Void
    /// The camera went away mid-scan (another app took it, or it was restricted).
    let onUnavailable: () -> Void

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .balanced,
            recognizesMultipleItems: false,
            isHighFrameRateTrackingEnabled: false,
            isPinchToZoomEnabled: true,
            isGuidanceEnabled: true,
            isHighlightingEnabled: true)
        scanner.delegate = context.coordinator
        return scanner
    }

    func updateUIViewController(_ scanner: DataScannerViewController, context: Context) {
        context.coordinator.onCode = onCode
        context.coordinator.onUnavailable = onUnavailable
        guard !scanner.isScanning else { return }
        do {
            try scanner.startScanning()
        } catch {
            onUnavailable()
        }
    }

    static func dismantleUIViewController(
        _ scanner: DataScannerViewController, coordinator: Coordinator
    ) {
        scanner.stopScanning()
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(onCode: onCode, onUnavailable: onUnavailable)
    }

    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        var onCode: (String) -> Void
        var onUnavailable: () -> Void
        /// The scanner re-adds a code that drifts out of frame and back.
        private var lastPayload: String?

        init(onCode: @escaping (String) -> Void, onUnavailable: @escaping () -> Void) {
            self.onCode = onCode
            self.onUnavailable = onUnavailable
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController, didAdd addedItems: [RecognizedItem],
            allItems: [RecognizedItem]
        ) {
            for case .barcode(let barcode) in addedItems {
                guard let payload = barcode.payloadStringValue, payload != lastPayload else {
                    continue
                }
                lastPayload = payload
                onCode(payload)
                return
            }
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController,
            becameUnavailableWithError error: DataScannerViewController.ScanningUnavailable
        ) {
            onUnavailable()
        }
    }
}
