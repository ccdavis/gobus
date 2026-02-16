// GoBus - Minimal JS for browser APIs
// Handles: geolocation, service worker, idle timeout, install prompt, saved locations

(function () {
  'use strict';

  // --- Service Worker Registration ---
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js', { scope: '/' })
      .then(function () { console.log('SW registered'); })
      .catch(function (err) { console.warn('SW registration failed:', err); });
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
    var html = '<div class="saved-locations-bar">';
    html += '<span class="saved-label">Saved:</span>';
    for (var i = 0; i < locs.length; i++) {
      var loc = locs[i];
      var href = '/nearby?lat=' + encodeURIComponent(loc.lat) +
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
  var manualEntry = document.getElementById('manual-entry');

  if (nearbyForm && latInput && lonInput) {
    // Check if lat/lon already provided (e.g. from manual entry or saved location)
    if (!(latInput.value && lonInput.value)) {
      if ('geolocation' in navigator) {
        if (locationStatus) {
          locationStatus.textContent = 'Finding your location\u2026';
        }

        navigator.geolocation.getCurrentPosition(
          function (pos) {
            latInput.value = pos.coords.latitude;
            lonInput.value = pos.coords.longitude;
            if (locationStatus) {
              locationStatus.textContent = 'Location found.';
            }
            // Auto-submit the form via HTMX if available
            if (window.htmx) {
              htmx.trigger(nearbyForm, 'submit');
            } else {
              nearbyForm.submit();
            }
          },
          function (err) {
            if (locationStatus) {
              if (err.code === 1) {
                // PERMISSION_DENIED
                locationStatus.textContent =
                  'Location is blocked by your browser. You can enable it in your browser settings, or enter an address below.';
              } else {
                locationStatus.textContent =
                  'Could not determine your location. Enter an address or intersection below.';
              }
            }
            if (manualEntry) {
              manualEntry.removeAttribute('hidden');
            }
          },
          { enableHighAccuracy: false, timeout: 10000, maximumAge: 60000 }
        );
      } else {
        // Geolocation not supported
        if (locationStatus) {
          locationStatus.textContent = 'Location services not available. Enter an address below.';
        }
        if (manualEntry) {
          manualEntry.removeAttribute('hidden');
        }
      }
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
