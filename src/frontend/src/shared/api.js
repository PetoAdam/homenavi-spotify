function resolveIntegrationBasePath() {
  if (typeof window === 'undefined') return '';
  const path = window.location.pathname || '';
  const parts = path.split('/').filter(Boolean);
  const idx = parts.indexOf('integrations');
  if (idx >= 0 && parts[idx + 1]) {
    return `/${['integrations', parts[idx + 1]].join('/')}`;
  }
  return '';
}

function buildUrl(path) {
  const base = resolveIntegrationBasePath();
  if (!path.startsWith('/')) return `${base}/${path}`;
  return `${base}${path}`;
}

export function buildIntegrationUrl(path) {
	return buildUrl(path);
}

async function jsonRequest(path, options = {}) {
  const resp = await fetch(buildUrl(path), options);
  if (resp.status === 204) return null;
  const text = await resp.text();
  const contentType = resp.headers.get('content-type') || '';
  if (!resp.ok) {
    let message = text || `Request failed (${resp.status})`;
    if (contentType.includes('application/json') && text) {
      try {
        const parsed = JSON.parse(text);
        message = parsed?.error || parsed?.message || message;
      } catch {
        // Fall back to raw text below.
      }
    } else if (/^\s*</.test(text)) {
      message = `Request failed (${resp.status})`;
    }
    const error = new Error(message);
    error.status = resp.status;
    error.body = text;
    error.contentType = contentType;
    throw error;
  }
  if (!text) return null;
  if (!contentType.includes('application/json')) {
    return text;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function getState() {
  return jsonRequest('/api/state');
}

export function getQueue() {
  return jsonRequest('/api/queue');
}

export function getDevices() {
  return jsonRequest('/api/devices');
}

export function play(payload = {}) {
  return jsonRequest('/api/play', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function pause() {
  return jsonRequest('/api/pause', { method: 'POST' });
}

export function nextTrack() {
  return jsonRequest('/api/next', { method: 'POST' });
}

export function previousTrack() {
  return jsonRequest('/api/previous', { method: 'POST' });
}

export function setShuffle(state) {
  return jsonRequest('/api/shuffle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ state }),
  });
}

export function setRepeat(state) {
  return jsonRequest('/api/repeat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ state }),
  });
}

export function setVolume(volumePercent) {
  return jsonRequest('/api/volume', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ volume_percent: volumePercent }),
  });
}

export function seek(positionMs) {
  return jsonRequest('/api/seek', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ position_ms: positionMs }),
  });
}

export function addToQueue(uri, deviceId) {
  return jsonRequest('/api/queue/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ uri, device_id: deviceId }),
  });
}

export function transferPlayback(deviceId, playOnTransfer = true) {
  return jsonRequest('/api/transfer', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_id: deviceId, play: playOnTransfer }),
  });
}

export function searchTracks(query) {
  const params = new URLSearchParams({ q: query });
  return jsonRequest(`/api/search?${params.toString()}`);
}

export function fetchSetup() {
  return jsonRequest('/api/admin/setup', { method: 'GET' });
}

export function saveSetup(settings) {
  return jsonRequest('/api/admin/setup', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ settings }),
  });
}

export function fetchAuthStatus() {
  return jsonRequest('/api/admin/auth/status', { method: 'GET' });
}

export function startSpotifyLogin(payload) {
  return jsonRequest('/api/admin/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function disconnectSpotify() {
  return jsonRequest('/api/admin/auth/disconnect', { method: 'POST' });
}
