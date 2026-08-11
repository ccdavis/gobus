import Foundation
import GobusKit

/// Manages the on-device Go core: copies the bundled SQLite schedule database to
/// a writable location on first launch, then starts the local HTTP server and
/// records the OS-assigned port.
///
/// The Go API (gomobile-bound from the `mobile` package) exposes three free
/// functions: `MobileStart(dbPath, dataDir, port, &boundPort, &error) -> Bool`,
/// `MobileStop()`, and `MobileRefresh()`.
enum GobusServer {
    private(set) static var port: Int = 0

    struct StartError: Error {
        let message: String
    }

    /// Brings up the server and returns the bound port. Failures are returned
    /// as errors so the UI can explain and offer a retry; all failure paths
    /// leave state untouched, so calling start() again is always safe.
    static func start() throws -> Int {
        if port > 0 { return port }

        let fm = FileManager.default
        let appSupport: URL
        do {
            appSupport = try fm.url(
                for: .applicationSupportDirectory, in: .userDomainMask,
                appropriateFor: nil, create: true)
        } catch {
            NSLog("GoBus: could not locate Application Support directory: \(error)")
            throw StartError(message: "Couldn’t access app storage.")
        }

        let dir = appSupport.appendingPathComponent("gobus", isDirectory: true)
        let dataDir = dir.appendingPathComponent("data", isDirectory: true)
        do {
            try fm.createDirectory(at: dir, withIntermediateDirectories: true)
            // Writable scratch dir for background GTFS refresh downloads.
            try fm.createDirectory(at: dataDir, withIntermediateDirectories: true)
        } catch {
            NSLog("GoBus: could not create data directories: \(error)")
            throw StartError(message: "Couldn’t create app storage.")
        }

        // First-launch DB copy: the app bundle is read-only and SQLite's WAL
        // needs a writable directory, so copy the prebuilt DB out of the bundle.
        // This happens once and is never overwritten — gobus.db also holds user
        // settings (saved locations, unit preference), so a re-copy on app update
        // would wipe them. Schedule data is kept current by the conditional
        // refresh the Go core runs at startup and on each foreground (iOS
        // suspends the process, so a timer-based background refresh can't be
        // relied on). Copy via a temp path + rename so an interrupted copy
        // can't leave a half-written file we'd later mistake for a valid DB.
        let dbURL = dir.appendingPathComponent("gobus.db")
        if !fm.fileExists(atPath: dbURL.path) {
            guard let bundled = Bundle.main.url(forResource: "gobus", withExtension: "db") else {
                NSLog("GoBus: bundled gobus.db not found in app bundle")
                throw StartError(message: "Schedule data is missing from the app.")
            }
            let tmpURL = dir.appendingPathComponent("gobus.db.copying")
            try? fm.removeItem(at: tmpURL)
            do {
                try fm.copyItem(at: bundled, to: tmpURL)
                try fm.moveItem(at: tmpURL, to: dbURL)
            } catch {
                try? fm.removeItem(at: tmpURL)
                NSLog("GoBus: failed to copy bundled DB: \(error)")
                throw StartError(message: "Couldn’t prepare schedule data.")
            }
        }

        var bound: Int = 0
        var startError: NSError?
        let ok = MobileStart(dbURL.path, dataDir.path, 0, &bound, &startError)
        if !ok || startError != nil {
            NSLog("GoBus: MobileStart failed: \(String(describing: startError))")
            throw StartError(message: "The GoBus engine couldn’t start.")
        }

        port = bound
        NSLog("GoBus: server started on 127.0.0.1:\(bound)")
        return bound
    }

    /// Kicks a conditional schedule-data refresh in the Go core (no-op when
    /// data is fresh). Called when the app returns to the foreground.
    static func refreshIfStale() {
        guard port > 0 else { return }
        MobileRefresh()
    }

    static func stop() {
        guard port > 0 else { return }
        MobileStop()
        port = 0
    }
}
