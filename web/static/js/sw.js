// GoBus Service Worker
var CACHE_SHELL = 'gobus-shell-v2';
var CACHE_PAGES = 'gobus-pages-v2';

var SHELL_ASSETS = [
  '/static/css/main.css',
  '/static/js/htmx.min.js',
  '/static/js/htmx-sse.js',
  '/static/js/app.js',
  '/static/icons/icon-192.png',
  '/offline',
  '/manifest.json'
];

// Max number of pages to keep cached
var MAX_CACHED_PAGES = 30;

// Install: pre-cache the app shell
self.addEventListener('install', function (event) {
  event.waitUntil(
    caches.open(CACHE_SHELL).then(function (cache) {
      return cache.addAll(SHELL_ASSETS);
    })
  );
  self.skipWaiting();
});

// Activate: clean up old caches
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

// Fetch: shell assets cache-first, pages network-first with offline fallback
self.addEventListener('fetch', function (event) {
  var url = new URL(event.request.url);

  // Only handle GET requests
  if (event.request.method !== 'GET') return;

  // Skip SSE connections
  if (event.request.headers.get('Accept') === 'text/event-stream') return;

  // Skip HTMX partial requests (only cache full page loads)
  if (event.request.headers.get('HX-Request') === 'true') return;

  // Shell assets: cache-first
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

  // HTML pages: network-first, cache fallback, then offline page
  if (event.request.headers.get('Accept') &&
      event.request.headers.get('Accept').indexOf('text/html') !== -1) {
    event.respondWith(
      fetch(event.request)
        .then(function (response) {
          // Only cache successful responses
          if (response.ok) {
            var clone = response.clone();
            caches.open(CACHE_PAGES).then(function (cache) {
              cache.put(event.request, clone);
              // Trim old pages if we have too many
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

// Trim cache to maxItems by removing oldest entries
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
