// GoBus - Minimal JS for browser APIs
// Handles: geolocation, service worker, idle timeout, install prompt, saved locations

(function () {
  'use strict';

  // Native app shell (iOS WKWebView): the server marks the page with
  // data-native. PWA behavior — service worker, page caching, install
  // prompts — is browser-only and must not run inside the installed app
  // (each native launch uses a random local port, i.e. a different origin,
  // so caches would fragment and never be cleaned up).
  var isNativeShell = document.documentElement.hasAttribute('data-native');

  // --- Service Worker Registration ---
  if (!isNativeShell && 'serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js', { scope: '/' })
      .then(function () { console.log('SW registered'); })
      .catch(function (err) { console.warn('SW registration failed:', err); });

    // Force Safari (and all browsers) to check for SW updates on every page load
    if (navigator.serviceWorker.controller) {
      navigator.serviceWorker.ready.then(function (reg) { reg.update(); });
    }
  }

  // --- Saved Locations (persisted in the local SQLite DB via /api/saved) ---
  // Loaded once into an in-memory cache; mutations hit the API then update it.
  var LEGACY_STORAGE_KEY = 'gobus-saved-locations'; // migrated once, then removed
  var savedLocations = [];

  function getSavedLocations() {
    return savedLocations;
  }

  function apiGetSaved() {
    return fetch('/api/saved', { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.ok ? r.json() : []; })
      .catch(function () { return []; });
  }

  function apiAddSaved(loc) {
    return fetch('/api/saved', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams(loc).toString()
    });
  }

  function addSavedLocation(loc) {
    // Don't duplicate by stopID
    for (var i = 0; i < savedLocations.length; i++) {
      if (savedLocations[i].stopID === loc.stopID) return Promise.resolve(false);
    }
    return apiAddSaved(loc).then(function (r) {
      if (r && r.ok) { savedLocations.push(loc); return true; }
      return false;
    }).catch(function () { return false; });
  }

  function removeSavedLocation(stopID) {
    return fetch('/api/saved/' + encodeURIComponent(stopID), { method: 'DELETE' })
      .then(function (r) {
        // Only drop the local copy once the server confirmed the delete;
        // otherwise the UI would claim a removal that didn't happen.
        if (r && r.ok) {
          savedLocations = savedLocations.filter(function (l) { return l.stopID !== stopID; });
          return true;
        }
        announceSavedError('Couldn’t remove the saved location. Please try again.');
        return false;
      }).catch(function () {
        announceSavedError('Couldn’t remove the saved location. Please try again.');
        return false;
      });
  }

  function isSaved(stopID) {
    for (var i = 0; i < savedLocations.length; i++) {
      if (savedLocations[i].stopID === stopID) return true;
    }
    return false;
  }

  // Announce a saved-locations failure to all users (visible + role=alert)
  // instead of silently swallowing it.
  function announceSavedError(msg) {
    var el = document.getElementById('saved-error');
    if (!el) {
      el = document.createElement('div');
      el.id = 'saved-error';
      el.className = 'saved-error';
      el.setAttribute('role', 'alert');
      var container = document.getElementById('saved-locations');
      if (container) {
        container.removeAttribute('hidden');
        container.appendChild(el);
      } else {
        document.body.appendChild(el);
      }
    }
    el.textContent = msg;
  }

  // One-time migration of any locations left in localStorage into the DB.
  // localStorage is only cleared after every record has been confirmed
  // written — until then it remains the durable copy, so a failed migration
  // retries on a later page load instead of losing the data.
  function migrateLegacySaved() {
    if (savedLocations.length) return Promise.resolve();
    var raw;
    try { raw = localStorage.getItem(LEGACY_STORAGE_KEY); } catch (e) { return Promise.resolve(); }
    if (!raw) return Promise.resolve();
    var legacy;
    try { legacy = JSON.parse(raw); } catch (e) { return Promise.resolve(); }
    if (!legacy || !legacy.length) return Promise.resolve();

    return Promise.all(legacy.map(function (loc) {
      return apiAddSaved(loc)
        .then(function (r) {
          if (r && r.ok) { savedLocations.push(loc); return true; }
          return false;
        })
        .catch(function () { return false; });
    })).then(function (results) {
      var allOK = results.every(function (ok) { return ok; });
      if (allOK) {
        try { localStorage.removeItem(LEGACY_STORAGE_KEY); } catch (e) { /* ignore */ }
      }
    });
  }

  // Render saved location buttons on the nearby page
  function renderSavedLocations() {
    var container = document.getElementById('saved-locations');
    if (!container) return;

    var locs = getSavedLocations();
    if (locs.length === 0) {
      container.setAttribute('hidden', '');
      return;
    }

    container.removeAttribute('hidden');
    var currentView = new URLSearchParams(window.location.search).get('view') || 'routes';
    var html = '<div class="saved-locations-bar">';
    html += '<span class="saved-label">Saved:</span>';
    for (var i = 0; i < locs.length; i++) {
      var loc = locs[i];
      var href = '/nearby?view=' + encodeURIComponent(currentView) +
                 '&lat=' + encodeURIComponent(loc.lat) +
                 '&lon=' + encodeURIComponent(loc.lon);
      html += '<a href="' + href + '" class="saved-btn" title="' +
              loc.name.replace(/"/g, '&quot;') + '">' +
              escapeHtml(loc.label || loc.name) + '</a>';
    }
    html += '<a href="#" id="manage-saved-btn" class="saved-manage" aria-label="Manage saved locations">Edit</a>';
    html += '</div>';
    container.innerHTML = html;
  }

  function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  // Manage saved locations dialog (inline)
  function showManageSaved() {
    var container = document.getElementById('saved-locations');
    if (!container) return;

    var locs = getSavedLocations();
    if (locs.length === 0) {
      renderSavedLocations();
      return;
    }

    var html = '<div class="saved-manage-panel">';
    html += '<h3>Saved Locations</h3>';
    html += '<ul role="list" style="list-style:none;padding:0;margin:0">';
    for (var i = 0; i < locs.length; i++) {
      var loc = locs[i];
      html += '<li class="saved-manage-item">';
      html += '<a href="/stops/' + encodeURIComponent(loc.stopID) + '">' +
              escapeHtml(loc.name) + '</a>';
      if (loc.label && loc.label !== loc.name) {
        html += ' <span class="distance">(' + escapeHtml(loc.label) + ')</span>';
      }
      html += ' <button class="btn-small btn-secondary remove-saved-btn" ' +
              'data-stop-id="' + loc.stopID + '" aria-label="Remove ' +
              loc.name.replace(/"/g, '&quot;') + '">Remove</button>';
      html += '</li>';
    }
    html += '</ul>';
    html += '<button id="done-manage-btn" class="btn-small">Done</button>';
    html += '</div>';
    container.innerHTML = html;
  }

  // Event delegation for saved locations
  document.addEventListener('click', function (e) {
    // "Edit" link to manage saved locations
    if (e.target && e.target.id === 'manage-saved-btn') {
      e.preventDefault();
      showManageSaved();
      return;
    }

    // "Done" managing saved locations
    if (e.target && e.target.id === 'done-manage-btn') {
      renderSavedLocations();
      return;
    }

    // Remove a saved location
    if (e.target && e.target.classList.contains('remove-saved-btn')) {
      var stopID = e.target.getAttribute('data-stop-id');
      if (stopID) {
        removeSavedLocation(stopID).then(showManageSaved);
      }
      return;
    }
  });

  // Load saved locations from the DB, then render and sync the save button.
  apiGetSaved().then(function (list) {
    savedLocations = Array.isArray(list) ? list : [];
    return migrateLegacySaved();
  }).then(function () {
    renderSavedLocations();
    updateSaveStopButton();
  });

  // --- Direction Toggle ---
  // Works for both route-nearby-row clicks and direction-toggle button clicks
  document.addEventListener('click', function (e) {
    // Don't toggle if clicking the later link
    if (e.target.closest('.later-link')) return;

    // Check for route row click or direction-toggle button click
    var row = e.target.closest('.route-nearby-row');
    var btn = e.target.closest('.direction-toggle');
    var group = null;

    if (row) {
      group = row.querySelector('.direction-group');
    } else if (btn) {
      group = btn.closest('.direction-group');
    }

    if (!group) return;

    var primary = group.querySelector('.direction-primary');
    var alt = group.querySelector('.direction-alt');
    if (!primary || !alt) return;

    var showingPrimary = !primary.hasAttribute('hidden');
    if (showingPrimary) {
      primary.setAttribute('hidden', '');
      alt.removeAttribute('hidden');
    } else {
      alt.setAttribute('hidden', '');
      primary.removeAttribute('hidden');
    }
  });

  // --- Save Stop Button (on stop detail page) ---
  // Reflects current saved state from the cache; re-queried so the async
  // initial load can call it once the saved list arrives.
  function updateSaveStopButton() {
    var btn = document.getElementById('save-stop-btn');
    if (!btn) return;
    var sid = btn.getAttribute('data-stop-id');
    var sname = btn.getAttribute('data-stop-name');
    if (isSaved(sid)) {
      btn.textContent = 'Saved';
      btn.setAttribute('aria-label', sname + ' is saved');
    } else {
      btn.textContent = 'Save stop';
      btn.setAttribute('aria-label', 'Save ' + sname + ' to your locations');
    }
  }

  var saveStopBtn = document.getElementById('save-stop-btn');
  if (saveStopBtn) {
    var stopID = saveStopBtn.getAttribute('data-stop-id');
    var stopName = saveStopBtn.getAttribute('data-stop-name');

    saveStopBtn.addEventListener('click', function () {
      if (isSaved(stopID)) {
        removeSavedLocation(stopID).then(updateSaveStopButton);
      } else {
        // Prompt for a short label
        var label = prompt('Give this location a short name (e.g. "Home", "Work"):', stopName);
        if (label === null) return; // cancelled
        if (label.trim() === '') label = stopName;

        addSavedLocation({
          stopID: stopID,
          name: stopName,
          label: label.trim(),
          lat: saveStopBtn.getAttribute('data-stop-lat'),
          lon: saveStopBtn.getAttribute('data-stop-lon')
        }).then(updateSaveStopButton);
      }
    });
  }

  // --- Geolocation ---
  var nearbyForm = document.getElementById('nearby-form');
  var latInput = document.getElementById('lat');
  var lonInput = document.getElementById('lon');
  var locationStatus = document.getElementById('location-status');

  if (nearbyForm && latInput && lonInput) {
    var currentView = new URLSearchParams(window.location.search).get('view') || 'routes';
    var searchURL = '/search?view=' + encodeURIComponent(currentView);

    // Approximate straight-line distance in meters between two lat/lon points
    function approxDistMeters(lat1, lon1, lat2, lon2) {
      var dLat = (lat2 - lat1) * 111320;
      var dLon = (lon2 - lon1) * 111320 * Math.cos(lat1 * Math.PI / 180);
      return Math.sqrt(dLat * dLat + dLon * dLon);
    }

    var params = new URLSearchParams(window.location.search);
    var hasQuery = params.get('q');
    var isGeoSource = params.get('src') === 'geo';

    // Helper: ensure src=geo hidden input exists on the form
    function ensureGeoSrc() {
      if (!nearbyForm.querySelector('input[name="src"]')) {
        var inp = document.createElement('input');
        inp.type = 'hidden';
        inp.name = 'src';
        inp.value = 'geo';
        nearbyForm.appendChild(inp);
      }
    }

    if (!(latInput.value && lonInput.value)) {
      // No coordinates yet — run geolocation and submit
      if ('geolocation' in navigator) {
        if (locationStatus) {
          locationStatus.textContent = 'Finding your location\u2026';
        }

        navigator.geolocation.getCurrentPosition(
          function (pos) {
            latInput.value = pos.coords.latitude;
            lonInput.value = pos.coords.longitude;
            ensureGeoSrc();
            if (locationStatus) {
              locationStatus.textContent = 'Location found.';
            }
            nearbyForm.submit();
          },
          function (err) {
            if (locationStatus) {
              if (err.code === 1) {
                locationStatus.innerHTML =
                  'Location is blocked by your browser. <a href="' + searchURL + '">Search for a location</a> instead, or enable location in browser settings.';
              } else {
                locationStatus.innerHTML =
                  'Could not determine your location. <a href="' + searchURL + '">Search for a location</a> instead.';
              }
            }
          },
          { enableHighAccuracy: false, timeout: 10000, maximumAge: 60000 }
        );
      } else {
        if (locationStatus) {
          locationStatus.innerHTML =
            'Location services not available. <a href="' + searchURL + '">Search for a location</a> instead.';
        }
      }
    } else if (isGeoSource && 'geolocation' in navigator) {
      // Coordinates came from geolocation (not saved location or search).
      // Background check: reload only if user moved >25m.
      navigator.geolocation.getCurrentPosition(
        function (pos) {
          var dist = approxDistMeters(
            parseFloat(latInput.value), parseFloat(lonInput.value),
            pos.coords.latitude, pos.coords.longitude
          );
          if (dist > 25) {
            latInput.value = pos.coords.latitude;
            lonInput.value = pos.coords.longitude;
            ensureGeoSrc();
            nearbyForm.submit();
          }
        },
        function () { /* ignore — keep current location */ },
        { enableHighAccuracy: false, timeout: 10000, maximumAge: 60000 }
      );
    }
  }

  // --- PWA Install Prompt ---
  var installPrompt = null;
  var installBanner = document.getElementById('install-banner');
  var installBtn = document.getElementById('install-btn');
  var installDismiss = document.getElementById('install-dismiss');

  window.addEventListener('beforeinstallprompt', function (e) {
    e.preventDefault();
    installPrompt = e;
    if (installBanner) {
      installBanner.removeAttribute('hidden');
    }
  });

  if (installBtn) {
    installBtn.addEventListener('click', function () {
      if (!installPrompt) return;
      installPrompt.prompt();
      installPrompt.userChoice.then(function () {
        installPrompt = null;
        if (installBanner) installBanner.setAttribute('hidden', '');
      });
    });
  }

  if (installDismiss) {
    installDismiss.addEventListener('click', function () {
      if (installBanner) installBanner.setAttribute('hidden', '');
      try {
        localStorage.setItem('gobus-install-dismissed', '1');
      } catch (e) { /* ignore */ }
    });
  }

  // Hide install banner if previously dismissed
  if (installBanner) {
    try {
      if (localStorage.getItem('gobus-install-dismissed') === '1') {
        installBanner.setAttribute('hidden', '');
      }
    } catch (e) { /* ignore */ }
  }

  // Hide install banner if already installed (standalone mode)
  if (window.matchMedia('(display-mode: standalone)').matches) {
    if (installBanner) installBanner.setAttribute('hidden', '');
  }

  // --- Distance Unit Toggle (persisted in the local SQLite DB) ---
  // The server renders the current unit into <html data-unit="…">, so there's
  // no flash and no localStorage; toggling persists via /api/settings/unit.
  function getDistanceUnit() {
    return document.documentElement.getAttribute('data-unit') === 'imperial' ? 'imperial' : 'metric';
  }

  function setDistanceUnit(unit) {
    document.documentElement.setAttribute('data-unit', unit);
    fetch('/api/settings/unit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: 'unit=' + encodeURIComponent(unit)
    }).catch(function () { /* ignore */ });
  }

  function formatDistText(meters, unit) {
    if (unit === 'imperial') {
      var miles = meters / 1609.344;
      if (miles < 0.1) {
        return Math.round(meters * 3.28084) + ' ft';
      }
      return miles.toFixed(1) + ' mi';
    }
    if (meters < 1000) {
      return Math.round(meters) + ' m';
    }
    return (meters / 1000).toFixed(1) + ' km';
  }

  function applyDistanceUnit() {
    var unit = getDistanceUnit();
    var els = document.querySelectorAll('[data-meters]');
    for (var i = 0; i < els.length; i++) {
      var meters = parseFloat(els[i].getAttribute('data-meters'));
      var walkMeters = parseFloat(els[i].getAttribute('data-walk-meters'));
      if (isNaN(meters)) continue;
      if (isNaN(walkMeters)) walkMeters = meters;
      var walkMin = Math.max(1, Math.round(walkMeters / 80.467));
      els[i].textContent = formatDistText(meters, unit) + ' (' + walkMin + ' min walk)';
    }
    var btn = document.getElementById('unit-toggle');
    if (btn) {
      if (unit === 'imperial') {
        btn.textContent = 'mi';
        btn.setAttribute('aria-label', 'Distance in miles. Click to switch to meters.');
      } else {
        btn.textContent = 'm';
        btn.setAttribute('aria-label', 'Distance in meters. Click to switch to miles.');
      }
    }
  }

  document.addEventListener('click', function (e) {
    if (e.target && e.target.id === 'unit-toggle') {
      var unit = getDistanceUnit() === 'metric' ? 'imperial' : 'metric';
      setDistanceUnit(unit);
      applyDistanceUnit();
    }
  });

  applyDistanceUnit();

  document.addEventListener('htmx:afterSwap', function () {
    applyDistanceUnit();
  });

  // --- Idle Timeout for SSE ---
  var IDLE_TIMEOUT_MS = 10 * 60 * 1000; // 10 minutes
  var idleTimer = null;

  // Track each element's live EventSource. The htmx SSE extension only closes
  // sources when the element is removed from the DOM (htmx:beforeCleanupElement)
  // — removing the sse-connect attribute does NOT close the underlying
  // connection, so we must close it ourselves on idle or connections pile up
  // across idle/wake cycles.
  document.body.addEventListener('htmx:sseOpen', function (e) {
    if (e.target && e.detail && e.detail.source) {
      e.target._gobusSSESource = e.detail.source;
    }
  });

  function resetIdleTimer() {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(onIdle, IDLE_TIMEOUT_MS);
  }

  function onIdle() {
    // Close all SSE connections: close the EventSource explicitly, then strip
    // the attribute so htmx doesn't reconnect until the user wakes the page.
    var sseElements = document.querySelectorAll('[sse-connect]');
    sseElements.forEach(function (el) {
      var url = el.getAttribute('sse-connect');
      el.removeAttribute('sse-connect');
      el.setAttribute('data-sse-was', url);
      if (el._gobusSSESource) {
        try { el._gobusSSESource.close(); } catch (e) { /* already closed */ }
        el._gobusSSESource = null;
      }
    });

    // Show wake-up banner and move focus to it (it has tabindex="-1")
    var banner = document.getElementById('idle-banner');
    if (banner) {
      banner.removeAttribute('hidden');
      banner.focus();
    }
  }

  // Wake up: reconnect SSE
  document.addEventListener('click', function (e) {
    if (e.target && e.target.id === 'wake-up-btn') {
      var banner = document.getElementById('idle-banner');
      if (banner) banner.setAttribute('hidden', '');

      // Restore SSE connections
      var elements = document.querySelectorAll('[data-sse-was]');
      elements.forEach(function (el) {
        el.setAttribute('sse-connect', el.getAttribute('data-sse-was'));
        el.removeAttribute('data-sse-was');
        if (window.htmx) htmx.process(el);
      });

      resetIdleTimer();
    }
  });

  // Reset idle timer on any user interaction
  ['click', 'keydown', 'scroll', 'touchstart'].forEach(function (evt) {
    document.addEventListener(evt, resetIdleTimer, { passive: true });
  });

  // Start idle timer
  resetIdleTimer();
})();
