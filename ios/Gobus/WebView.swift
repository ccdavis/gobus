import SwiftUI
import WebKit

/// A thin WKWebView wrapper that loads the local GoBus server. The UI itself is
/// the same server-rendered templ + HTMX app the desktop build serves.
///
/// Navigation policy: the web view may only browse the local GoBus origin.
/// Anything else — external links (e.g. "Show on Map"), `target="_blank"`
/// windows, unexpected hosts or schemes — is either handed to the system
/// (Safari/Maps) or rejected, so the shell can never be steered to foreign
/// content.
struct WebView: UIViewRepresentable {
    let url: URL

    func makeCoordinator() -> Coordinator {
        Coordinator(localURL: url)
    }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()

        let webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = context.coordinator
        webView.uiDelegate = context.coordinator
        webView.allowsBackForwardNavigationGestures = true
        webView.scrollView.contentInsetAdjustmentBehavior = .always
        webView.isOpaque = false
        webView.backgroundColor = .systemBackground
        webView.scrollView.backgroundColor = .systemBackground
        webView.load(URLRequest(url: url))
        return webView
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}

    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        private let localURL: URL

        init(localURL: URL) {
            self.localURL = localURL
        }

        private func isLocal(_ url: URL) -> Bool {
            url.scheme == "http" && url.host == localURL.host && url.port == localURL.port
        }

        /// Externally openable link schemes; everything else is dropped.
        private func openExternally(_ url: URL) {
            guard ["https", "http", "mailto", "tel", "maps"].contains(url.scheme ?? "") else {
                NSLog("GoBus: rejected navigation to unexpected scheme: \(url)")
                return
            }
            UIApplication.shared.open(url)
        }

        func webView(_ webView: WKWebView,
                     decidePolicyFor navigationAction: WKNavigationAction,
                     decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            guard let url = navigationAction.request.url else {
                decisionHandler(.cancel)
                return
            }
            // New-window targets (target="_blank") and non-local URLs leave
            // the shell and open in the system browser/app instead.
            if navigationAction.targetFrame == nil || !isLocal(url) {
                if isLocal(url) {
                    // A local link asking for a new window just navigates here.
                    webView.load(URLRequest(url: url))
                } else {
                    openExternally(url)
                }
                decisionHandler(.cancel)
                return
            }
            decisionHandler(.allow)
        }

        // target="_blank" flows that bypass decidePolicyFor's nil-targetFrame
        // path (e.g. window.open) land here; open externally, never create a
        // second web view.
        func webView(_ webView: WKWebView,
                     createWebViewWith configuration: WKWebViewConfiguration,
                     for navigationAction: WKNavigationAction,
                     windowFeatures: WKWindowFeatures) -> WKWebView? {
            if let url = navigationAction.request.url {
                if isLocal(url) {
                    webView.load(URLRequest(url: url))
                } else {
                    openExternally(url)
                }
            }
            return nil
        }

        // The local server should always answer; a failure here is transient
        // (e.g. the listener not accepting yet during a cold start), so retry
        // the app root after a short pause rather than leaving a blank view.
        func webView(_ webView: WKWebView,
                     didFailProvisionalNavigation navigation: WKNavigation!,
                     withError error: Error) {
            scheduleRecovery(webView, error: error)
        }

        func webView(_ webView: WKWebView,
                     didFail navigation: WKNavigation!,
                     withError error: Error) {
            scheduleRecovery(webView, error: error)
        }

        func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
            NSLog("GoBus: WebContent process terminated; reloading")
            webView.reload()
        }

        private func scheduleRecovery(_ webView: WKWebView, error: Error) {
            let nsError = error as NSError
            // Cancelled loads (our own decisionHandler(.cancel)) are not failures.
            if nsError.domain == NSURLErrorDomain && nsError.code == NSURLErrorCancelled {
                return
            }
            NSLog("GoBus: navigation failed (\(nsError.domain) \(nsError.code)); retrying")
            let target = localURL
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
                webView.load(URLRequest(url: target))
            }
        }
    }
}
