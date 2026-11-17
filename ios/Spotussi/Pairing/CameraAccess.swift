import AVFoundation
import Observation
import VisionKit

/// Whether the scanner can run: a camera that supports it, and permission.
@Observable
final class CameraAccess {
    nonisolated enum State: Equatable {
        case checking
        case ready
        /// Permission was refused, or is restricted on this device.
        case denied
        /// No camera that can scan: the simulator, or an unsupported device.
        case unavailable
    }

    private(set) var state: State = .checking

    func check() async {
        guard DataScannerViewController.isSupported else {
            state = .unavailable
            return
        }
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            state = DataScannerViewController.isAvailable ? .ready : .unavailable
        case .notDetermined:
            state = await AVCaptureDevice.requestAccess(for: .video) ? .ready : .denied
        case .denied, .restricted:
            state = .denied
        @unknown default:
            state = .unavailable
        }
    }

    func cameraWentAway() {
        state = .unavailable
    }
}
