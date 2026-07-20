import React, { useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import '../shared/hn.css';
import Player from '../shared/Player';
import {
  buildIntegrationUrl,
  fetchAuthStatus,
  fetchSetup,
  saveSetup,
  disconnectSpotify,
} from '../shared/api';

function isSetupPath() {
  if (typeof window === 'undefined') return false;
  const path = (window.location.pathname || '').toLowerCase();
  return path.endsWith('/ui/setup') || path.includes('/ui/setup/');
}

function formatExpiryLabel(isoValue, now) {
  if (!isoValue) return 'Not set';
  const target = Date.parse(isoValue);
  if (Number.isNaN(target)) return isoValue;
  const diff = target - now;
  if (diff <= 0) return 'Expired';
  const totalMinutes = Math.floor(diff / 60000);
  const days = Math.floor(totalMinutes / (60 * 24));
  const hours = Math.floor((totalMinutes % (60 * 24)) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) {
    return hours > 0 ? `${days}d ${hours}h remaining` : `${days}d remaining`;
  }
  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m remaining` : `${hours}h remaining`;
  }
  return `${minutes}m remaining`;
}

function SetupApp() {
  const [settings, setSettings] = React.useState({ client_id: '', client_secret: '' });
  const [authStatus, setAuthStatus] = React.useState({ connected: false });
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [status, setStatus] = React.useState('');
  const [nowTick, setNowTick] = React.useState(Date.now());

  const load = React.useCallback(async () => {
    const [setupResp, authResp] = await Promise.all([fetchSetup(), fetchAuthStatus()]);
    setSettings({
      client_id: String(setupResp?.settings?.client_id || ''),
      client_secret: String(setupResp?.settings?.client_secret || ''),
    });
    setAuthStatus(authResp?.status || { connected: false });
  }, []);

  React.useEffect(() => {
    let alive = true;
    load()
      .catch((err) => {
        if (!alive) return;
        setStatus(err?.message || 'Could not load setup.');
      })
      .finally(() => {
        if (alive) setLoading(false);
      });

    const params = new URLSearchParams(window.location.search);
    const spotify = params.get('spotify');
    if (spotify === 'connected') {
      setStatus('Spotify account connected.');
    }
    if (spotify === 'auth_error') {
      setStatus('Spotify login failed. Try connecting again.');
    }
    return () => {
      alive = false;
    };
  }, [load]);

  React.useEffect(() => {
    const timer = window.setInterval(() => setNowTick(Date.now()), 60000);
    return () => window.clearInterval(timer);
  }, []);

  const expiryLabel = formatExpiryLabel(authStatus?.refresh_token_expires_at, nowTick);

  // Build the direct-navigation URL for the OAuth begin endpoint.
  // Using a plain <a target="_top"> avoids the async user-gesture gap that breaks
  // window.top.location.href inside an iframe sandbox (allow-top-navigation-by-user-activation).
  const connectHref = React.useMemo(() => {
    const origin = window.location.origin;
    const redirectUri = encodeURIComponent(`${origin}${buildIntegrationUrl('/api/admin/auth/callback')}`);
    const returnTo = encodeURIComponent(`${origin}${buildIntegrationUrl('/ui/setup/')}`);
    return `${buildIntegrationUrl('/api/admin/auth/begin')}?redirect_uri=${redirectUri}&return_to=${returnTo}`;
  }, []);

  return (
    <div className="hn-shell">
      <div className="hn-card">
        <div className="hn-row">
          <div>
            <h1 className="hn-title">Spotify Setup</h1>
            <div className="hn-subtitle">Configure the Spotify app and connect the household Spotify account.</div>
          </div>
          <span className="hn-pill">setup</span>
        </div>

        <div className="hn-section">
          <div className="spotify-setup-note">Use this redirect URI in the Spotify developer dashboard:</div>
          <div className="spotify-setup-uri">{window.location.origin}{buildIntegrationUrl('/api/admin/auth/callback')}</div>

          <div style={{ display: 'grid', gap: 12, marginTop: 14 }}>
            <label style={{ display: 'grid', gap: 6 }}>
              <span className="hn-subtitle">Spotify Client ID</span>
              <input
                className="hn-input"
                value={settings.client_id}
                onChange={(e) => setSettings((prev) => ({ ...prev, client_id: e.target.value }))}
                placeholder="Spotify developer app client id"
                disabled={loading || saving}
              />
            </label>
            <label style={{ display: 'grid', gap: 6 }}>
              <span className="hn-subtitle">Spotify Client Secret</span>
              <input
                className="hn-input"
                type="password"
                value={settings.client_secret}
                onChange={(e) => setSettings((prev) => ({ ...prev, client_secret: e.target.value }))}
                placeholder="Spotify developer app client secret"
                disabled={loading || saving}
              />
            </label>
          </div>

          <div className="spotify-setup-meta">
            <div className="hn-subtitle">Connection status: {authStatus?.connected ? 'Connected' : 'Not connected'}</div>
            <div className="hn-subtitle">Refresh token: {expiryLabel}</div>
            {authStatus?.refresh_token_expires_at ? (
              <div className="hn-subtitle">Expires at: {authStatus.refresh_token_expires_at}</div>
            ) : null}
          </div>

          <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
            <button
              className="hn-btn"
              disabled={loading || saving}
              onClick={async () => {
                setSaving(true);
                setStatus('');
                try {
                  await saveSetup({
                    client_id: String(settings.client_id || '').trim(),
                    client_secret: String(settings.client_secret || '').trim(),
                  });
                  await load();
                  setStatus('Setup saved.');
                } catch (err) {
                  setStatus(err?.message || 'Failed to save setup.');
                } finally {
                  setSaving(false);
                }
              }}
            >
              {saving ? 'Saving…' : 'Save setup'}
            </button>
            <a
              className="hn-btn"
              href={connectHref}
              target="_top"
              style={{ pointerEvents: (loading || saving) ? 'none' : 'auto', opacity: (loading || saving) ? 0.5 : 1 }}
            >
              {authStatus?.connected ? 'Reconnect Spotify' : 'Connect Spotify'}
            </a>
            {authStatus?.connected ? (
              <button
                className="hn-btn"
                disabled={loading || saving}
                onClick={async () => {
                  try {
                    await disconnectSpotify();
                    await load();
                    setStatus('Spotify connection removed.');
                  } catch (err) {
                    setStatus(err?.message || 'Failed to disconnect Spotify.');
                  }
                }}
                >
                Disconnect
              </button>
                ) : null}
          </div>
          {status ? <div className="hn-subtitle" style={{ marginTop: 10 }}>{status}</div> : null}
        </div>
      </div>
    </div>
  );
}

function TabApp() {
  useEffect(() => {
    document.body.classList.add('hn-tab');
    return () => {
      document.body.classList.remove('hn-tab');
    };
  }, []);

  return (
    <div className="hn-shell hn-tab-shell">
      {isSetupPath() ? <SetupApp /> : <Player variant="tab" showSearch showQueue />}
    </div>
  );
}

createRoot(document.getElementById('root')).render(<TabApp />);
