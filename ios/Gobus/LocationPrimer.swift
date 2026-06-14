import CoreLocation

/// Requests When-In-Use location authorization at launch. WKWebView on iOS 15+
/// bridges the JS Geolocation API (used by `web/static/js/app.js`) to the host
/// app's CoreLocation permission, so priming the prompt here is enough for the
/// in-page `navigator.geolocation` calls to resolve.
final class LocationPrimer: NSObject, CLLocationManagerDelegate {
    static let shared = LocationPrimer()
    private let manager = CLLocationManager()

    func request() {
        manager.delegate = self
        if manager.authorizationStatus == .notDetermined {
            manager.requestWhenInUseAuthorization()
        }
    }

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        NSLog("GoBus: location authorization = \(manager.authorizationStatus.rawValue)")
    }
}
