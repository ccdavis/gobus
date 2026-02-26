// GoBus - Minimal JS for browser APIs
// Handles: geolocation, service worker, idle timeout, install prompt, saved locations

(function () {
  'use strict';

  // --- Service Worker Registration ---
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js', { scope: '/' })
      .then(function () { console.log('SW registered'); })
      .catch(function (err) { console.warn('SW registration failed:', err); });

    // Force Safari (and all browsers) to check for SW updates on every page load
    if (navigator.serviceWorker.controller) {
      navigator.serviceWorker.ready.then(function (reg) { reg.update(); });
    }
  }

  // --- Saved Locations ---
  var STORAGE_KEY = 'gobus-saved-locations';

  function getSavedLocations() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (raw) return JSON.parse(raw);
    } catch (e) { /* ignore */ }
    return [];
  }

  function saveSavedLocations(locs) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(locs));
    } catch (e) { /* ignore */ }
  }

  function addSavedLocation(loc) {
    var locs = getSavedLocations();
    // Don't duplicate by stopID
    for (var i = 0; i < locs.length; i++) {
      if (locs[i].stopID === loc.stopID) return false;
    }
    locs.push(loc);
    saveSavedLocations(locs);
    return true;
  }

  function removeSavedLocation(stopID) {
    var locs = getSavedLocations();
    locs = locs.filter(function (l) { return l.stopID !== stopID; });
    saveSavedLocations(locs);
  }

  function isSaved(stopID) {
    var locs = getSavedLocations();
    for (var i = 0; i < locs.length; i++) {
      if (locs[i].stopID === stopID) return true;
    }
    return false;
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
        removeSavedLocation(stopID);
        showManageSaved();
      }
      return;
    }
  });

  // Render saved locations on nearby page load
  renderSavedLocations();

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
  var saveStopBtn = document.getElementById('save-stop-btn');
  if (saveStopBtn) {
    var stopID = saveStopBtn.getAttribute('data-stop-id');
    var stopName = saveStopBtn.getAttribute('data-stop-name');

    // Update button state based on whether already saved
    if (isSaved(stopID)) {
      saveStopBtn.textContent = 'Saved';
      saveStopBtn.setAttribute('aria-label', stopName + ' is saved');
    }

    saveStopBtn.addEventListener('click', function () {
      if (isSaved(stopID)) {
        removeSavedLocation(stopID);
        saveStopBtn.textContent = 'Save stop';
        saveStopBtn.setAttribute('aria-label', 'Save ' + stopName + ' to your locations');
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
        });
        saveStopBtn.textContent = 'Saved';
        saveStopBtn.setAttribute('aria-label', stopName + ' is saved');
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

  // --- Distance Unit Toggle ---
  var UNIT_KEY = 'gobus-distance-unit';

  function getDistanceUnit() {
    try { return localStorage.getItem(UNIT_KEY) || 'metric'; }
    catch (e) { return 'metric'; }
  }

  function setDistanceUnit(unit) {
    try { localStorage.setItem(UNIT_KEY, unit); }
    catch (e) { /* ignore */ }
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

  function resetIdleTimer() {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(onIdle, IDLE_TIMEOUT_MS);
  }

  function onIdle() {
    // Close all SSE connections by removing the sse-connect attributes
    var sseElements = document.querySelectorAll('[sse-connect]');
    sseElements.forEach(function (el) {
      var url = el.getAttribute('sse-connect');
      el.removeAttribute('sse-connect');
      el.setAttribute('data-sse-was', url);
    });

    // Show wake-up banner
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
