import SwiftUI

@main
struct GobusApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .preferredColorScheme(.dark)
                .ignoresSafeArea(.keyboard)
        }
    }
}

/// Owns app-level startup: primes the location prompt and brings up the Go core,
/// publishing the bound port for the WebView to load.
final class AppModel: ObservableObject {
    @Published var port: Int = 0

    init() {
        LocationPrimer.shared.request()
        port = GobusServer.start()
    }
}

struct ContentView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        if model.port > 0, let url = URL(string: "http://127.0.0.1:\(model.port)/") {
            WebView(url: url)
                .ignoresSafeArea(edges: .bottom)
        } else {
            VStack(spacing: 12) {
                ProgressView()
                Text("Starting GoBus…")
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.black)
        }
    }
}
