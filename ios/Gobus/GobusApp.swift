import SwiftUI

@main
struct GobusApp: App {
    @StateObject private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .ignoresSafeArea(.keyboard)
                .task { model.start() }
                .onChange(of: scenePhase) { phase in
                    // Keep bundled schedule data fresh: iOS suspends the
                    // process, so refresh rides on foregrounding instead of a
                    // background timer.
                    if phase == .active { GobusServer.refreshIfStale() }
                }
        }
    }
}

/// Owns app-level startup: brings up the Go core off the main thread (the
/// first-launch DB copy + open must not block launch), publishing the bound
/// port for the WebView to load. Location permission is NOT requested here —
/// the nearby page's geolocation call triggers the system prompt once the user
/// actually sees the app.
@MainActor
final class AppModel: ObservableObject {
    enum Phase: Equatable {
        case starting
        case running(port: Int)
        case failed(message: String)
    }

    @Published var phase: Phase = .starting
    private var starting = false

    func start() {
        guard !starting, !isRunning else { return }
        starting = true
        phase = .starting
        Task.detached(priority: .userInitiated) {
            let result: Result<Int, Error> = Result { try GobusServer.start() }
            await MainActor.run {
                self.starting = false
                switch result {
                case .success(let port):
                    self.phase = .running(port: port)
                case .failure(let error):
                    let message = (error as? GobusServer.StartError)?.message
                        ?? "GoBus couldn’t start."
                    self.phase = .failed(message: message)
                }
            }
        }
    }

    func retry() {
        start()
    }

    private var isRunning: Bool {
        if case .running = phase { return true }
        return false
    }
}

struct ContentView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        switch model.phase {
        case .running(let port):
            if let url = URL(string: "http://127.0.0.1:\(port)/") {
                WebView(url: url)
                    .ignoresSafeArea(edges: .bottom)
            } else {
                failure("Couldn’t reach GoBus.")
            }
        case .starting:
            VStack(spacing: 12) {
                ProgressView()
                Text("Starting GoBus…").foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(.systemBackground))
        case .failed(let message):
            failure(message)
        }
    }

    private func failure(_ text: String) -> some View {
        VStack(spacing: 16) {
            Text(text)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            Button("Try Again") { model.retry() }
                .buttonStyle(.borderedProminent)
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(.systemBackground))
    }
}
