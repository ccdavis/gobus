import Foundation
import GobusKit

/// Manages the on-device Go core: copies the bundled SQLite schedule database to
/// a writable location on first launch, then starts the local HTTP server and
/// records the OS-assigned port.
///
/// The Go API (gomobile-bound from the `mobile` package) exposes two free
/// functions: `MobileStart(dbPath, dataDir, port, &boundPort, &error) -> Bool`
/// and `MobileStop()`.
enum GobusServer {
    private(set) static var port: Int = 0

    /// Brings up the server and returns the bound port, or 0 on failure.
    @discardableResult
    static func start() -> Int {
        if port > 0 { return port }

        let fm = FileManager.default
        guard let appSupport = try? fm.url(
            for: .applicationSupportDirectory, in: .userDomainMask,
            appropriateFor: nil, create: true)
        else {
            NSLog("GoBus: could not locate Application Support directory")
            return 0
        }

        let dir = appSupport.appendingPathComponent("gobus", isDirectory: true)
        try? fm.createDirectory(at: dir, withIntermediateDirectories: true)

        // First-launch DB copy: the app bundle is read-only and SQLite's WAL
        // needs a writable directory, so copy the prebuilt DB out of the bundle.
        // This happens once and is never overwritten — gobus.db also holds user
        // settings (saved locations, unit preference), so a re-copy on app update
        // would wipe them; schedule data is kept current by the background GTFS
        // refresh instead. Copy via a temp path + rename so an interrupted copy
        // can't leave a half-written file we'd later mistake for a valid DB.
        let dbURL = dir.appendingPathComponent("gobus.db")
        if !fm.fileExists(atPath: dbURL.path) {
            guard let bundled = Bundle.main.url(forResource: "gobus", withExtension: "db") else {
                NSLog("GoBus: bundled gobus.db not found in app bundle")
                return 0
            }
            let tmpURL = dir.appendingPathComponent("gobus.db.copying")
            try? fm.removeItem(at: tmpURL)
            do {
                try fm.copyItem(at: bundled, to: tmpURL)
                try fm.moveItem(at: tmpURL, to: dbURL)
            } catch {
                try? fm.removeItem(at: tmpURL)
                NSLog("GoBus: failed to copy bundled DB: \(error)")
                return 0
            }
        }

        // Writable scratch dir for any background GTFS refresh downloads.
        let dataDir = dir.appendingPathComponent("data", isDirectory: true)
        try? fm.createDirectory(at: dataDir, withIntermediateDirectories: true)

        var bound: Int = 0
        var startError: NSError?
        let ok = MobileStart(dbURL.path, dataDir.path, 0, &bound, &startError)
        if !ok || startError != nil {
            NSLog("GoBus: MobileStart failed: \(String(describing: startError))")
            return 0
        }

        port = bound
        NSLog("GoBus: server started on 127.0.0.1:\(bound)")
        return bound
    }

    static func stop() {
        guard port > 0 else { return }
        MobileStop()
        port = 0
    }
}
