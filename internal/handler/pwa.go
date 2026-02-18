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

// ServiceWorker serves the service worker script with the asset version embedded.
// Cache-Control: no-cache ensures browsers always check for updates to sw.js.
func (h *Handler) ServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, serviceWorkerScript, h.version, h.version)
}

const serviceWorkerScript = `// GoBus Service Worker — auto-generated with asset version
var ASSET_VERSION = '%s';
var CACHE_SHELL = 'gobus-shell-' + ASSET_VERSION;
var CACHE_PAGES = 'gobus-pages-' + ASSET_VERSION;

var SHELL_ASSETS = [
  '/static/css/main.css?v=' + ASSET_VERSION,
  '/static/js/htmx.min.js?v=' + ASSET_VERSION,
  '/static/js/htmx-sse.js?v=' + ASSET_VERSION,
  '/static/js/app.js?v=' + ASSET_VERSION,
  '/static/icons/icon-192.png',
  '/offline',
  '/manifest.json'
];

var MAX_CACHED_PAGES = 30;

self.addEventListener('install', function (event) {
  event.waitUntil(
    caches.open(CACHE_SHELL).then(function (cache) {
      return cache.addAll(SHELL_ASSETS);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', function (event) {
  var expected = [CACHE_SHELL, CACHE_PAGES];
  event.waitUntil(
    caches.keys().then(function (names) {
      return Promise.all(
        names.filter(function (n) { return expected.indexOf(n) === -1; })
          .map(function (n) { return caches.delete(n); })
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', function (event) {
  var url = new URL(event.request.url);
  if (event.request.method !== 'GET') return;
  if (event.request.headers.get('Accept') === 'text/event-stream') return;
  if (event.request.headers.get('HX-Request') === 'true') return;

  if (url.pathname.startsWith('/static/') || url.pathname === '/manifest.json') {
    event.respondWith(
      caches.match(event.request).then(function (cached) {
        return cached || fetch(event.request).then(function (response) {
          var clone = response.clone();
          caches.open(CACHE_SHELL).then(function (cache) {
            cache.put(event.request, clone);
          });
          return response;
        });
      })
    );
    return;
  }

  if (event.request.headers.get('Accept') &&
      event.request.headers.get('Accept').indexOf('text/html') !== -1) {
    event.respondWith(
      fetch(event.request)
        .then(function (response) {
          if (response.ok) {
            var clone = response.clone();
            caches.open(CACHE_PAGES).then(function (cache) {
              cache.put(event.request, clone);
              trimCache(CACHE_PAGES, MAX_CACHED_PAGES);
            });
          }
          return response;
        })
        .catch(function () {
          return caches.match(event.request).then(function (cached) {
            return cached || caches.match('/offline');
          });
        })
    );
    return;
  }
});

function trimCache(cacheName, maxItems) {
  caches.open(cacheName).then(function (cache) {
    cache.keys().then(function (keys) {
      if (keys.length > maxItems) {
        cache.delete(keys[0]).then(function () {
          trimCache(cacheName, maxItems);
        });
      }
    });
  });
}
`

// Offline serves the offline fallback page.
func (h *Handler) Offline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Offline — GoBus</title>
<link rel="stylesheet" href="/static/css/main.css?v=%s">
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
</html>`, h.version)
}
