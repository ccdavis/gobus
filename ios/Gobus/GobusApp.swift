import SwiftUI

@main
struct GobusApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .preferredColorScheme(.dark)
                .ignoresSafeArea(.keyboard)
                .task { model.start() }
        }
    }
}

/// Owns app-level startup: primes the location prompt and brings up the Go core
/// off the main thread (the first-launch DB copy + open must not block launch),
/// publishing the bound port for the WebView to load.
@MainActor
final class AppModel: ObservableObject {
    enum Phase: Equatable {
        case starting
        case running(port: Int)
        case failed
    }

    @Published var phase: Phase = .starting
    private var didStart = false

    func start() {
        guard !didStart else { return }
        didStart = true
        LocationPrimer.shared.request()
        Task.detached(priority: .userInitiated) {
            let port = GobusServer.start()
            await MainActor.run {
                self.phase = port > 0 ? .running(port: port) : .failed
            }
        }
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
                message("Couldn’t reach GoBus.")
            }
        case .starting:
            VStack(spacing: 12) {
                ProgressView()
                Text("Starting GoBus…").foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.black)
        case .failed:
            message("GoBus couldn’t start.")
        }
    }

    private func message(_ text: String) -> some View {
        Text(text)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.black)
    }
}
