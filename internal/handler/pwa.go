package handler

import (
	"fmt"
	"net/http"
)

// Manifest serves the PWA manifest.
func (h *Handler) Manifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	fmt.Fprint(w, `{
  "name": "GoBus - Metro Transit",
  "short_name": "GoBus",
  "description": "Real-time Metro Transit departures for Minneapolis/St. Paul",
  "start_url": "/nearby",
  "scope": "/",
  "display": "standalone",
  "orientation": "any",
  "background_color": "#1a1a2e",
  "theme_color": "#1a1a2e",
  "categories": ["navigation", "transportation"],
  "icons": [
    {
      "src": "/static/icons/icon-192.png",
      "sizes": "192x192",
      "type": "image/png",
      "purpose": "any"
    },
    {
      "src": "/static/icons/icon-512.png",
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "any"
    },
    {
      "src": "/static/icons/icon-512.png",
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "maskable"
    }
  ],
  "shortcuts": [
    {
      "name": "Nearby Departures",
      "short_name": "Nearby",
      "url": "/nearby",
      "description": "Find departures near your current location"
    },
    {
      "name": "Route Explorer",
      "short_name": "Routes",
      "url": "/routes",
      "description": "Browse all Metro Transit routes"
    }
  ]
}`)
}

// ServiceWorker serves the service worker script.
func (h *Handler) ServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	http.ServeFile(w, r, "web/static/js/sw.js")
}

// Offline serves the offline fallback page.
func (h *Handler) Offline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Offline — GoBus</title>
<link rel="stylesheet" href="/static/css/main.css">
<meta name="theme-color" content="#1a1a2e">
</head>
<body>
<a href="#main" class="skip-link">Skip to main content</a>
<header role="banner">
<nav aria-label="Main navigation">
<a href="/nearby">Nearby</a>
<a href="/routes">Route Explorer</a>
</nav>
<h1>GoBus</h1>
</header>
<main id="main" role="main">
<h2>You're offline</h2>
<p>GoBus can't reach the server right now. Previously visited stops and routes may still be available — try the links above.</p>
<p>Realtime departure information requires an internet connection, but cached pages will show the last schedule data you viewed.</p>
<button onclick="location.reload()">Try Again</button>
</main>
<footer role="contentinfo">
<p>GoBus — Metro Transit departure info</p>
</footer>
</body>
</html>`)
}
