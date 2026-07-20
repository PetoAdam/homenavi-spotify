package backend

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const spotifyNotConfiguredMessage = "spotify integration is not configured"

func RegisterAPIRoutes(mux *http.ServeMux, spotifyManager *SpotifyManager, playback *PlaybackCache) {
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodGet, "/me/player", nil, nil)
		if status == http.StatusNoContent {
			if cached, ok := pausedCachedPlayback(playback); ok {
				writeRawJSON(w, http.StatusOK, cached)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"active": false})
			return
		}
		if err != nil {
			if isNoActiveDevice(body, err) {
				if cached, ok := pausedCachedPlayback(playback); ok {
					writeRawJSON(w, http.StatusOK, cached)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"active": false})
				return
			}
			writeSpotifyResponse(w, status, body, err)
			return
		}
		if len(body) > 0 {
			playback.Set(body)
		}
		writeRawJSON(w, status, body)
	})

	mux.HandleFunc("/api/queue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodGet, "/me/player/queue", nil, nil)
		writeSpotifyResponse(w, status, body, err)
	})

	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodGet, "/me/player/devices", nil, nil)
		writeSpotifyResponse(w, status, body, err)
	})

	mux.HandleFunc("/api/play", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			ContextURI string         `json:"context_uri"`
			URIs       []string       `json:"uris"`
			Offset     map[string]any `json:"offset"`
			PositionMS *int           `json:"position_ms"`
			DeviceID   string         `json:"device_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)

		body := map[string]any{}
		if payload.ContextURI != "" {
			body["context_uri"] = payload.ContextURI
		}
		if len(payload.URIs) > 0 {
			body["uris"] = payload.URIs
		}
		if payload.Offset != nil {
			body["offset"] = payload.Offset
		}
		if payload.PositionMS != nil {
			body["position_ms"] = *payload.PositionMS
		}

		query := url.Values{}
		if payload.DeviceID != "" {
			query.Set("device_id", payload.DeviceID)
		}

		status, respBody, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/play", query, body)
		writeSpotifyResponseWithCache(w, status, respBody, err, playback)
	})

	mux.HandleFunc("/api/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/pause", nil, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/next", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodPost, "/me/player/next", nil, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/previous", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		status, body, err := spotify.Do(r.Context(), http.MethodPost, "/me/player/previous", nil, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/shuffle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			State bool `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		query := url.Values{}
		query.Set("state", boolString(payload.State))
		status, body, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/shuffle", query, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/repeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if payload.State == "" {
			payload.State = "off"
		}
		query := url.Values{}
		query.Set("state", payload.State)
		status, body, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/repeat", query, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/volume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			VolumePercent int `json:"volume_percent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		query := url.Values{}
		query.Set("volume_percent", intString(payload.VolumePercent))
		status, body, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/volume", query, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/seek", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			PositionMS int `json:"position_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		query := url.Values{}
		query.Set("position_ms", intString(payload.PositionMS))
		status, body, err := spotify.Do(r.Context(), http.MethodPut, "/me/player/seek", query, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/queue/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			URI      string `json:"uri"`
			DeviceID string `json:"device_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if payload.URI == "" {
			writeJSONError(w, http.StatusBadRequest, "missing uri")
			return
		}
		query := url.Values{}
		query.Set("uri", payload.URI)
		if payload.DeviceID != "" {
			query.Set("device_id", payload.DeviceID)
		}
		status, body, err := spotify.Do(r.Context(), http.MethodPost, "/me/player/queue", query, nil)
		writeSpotifyResponseWithCache(w, status, body, err, playback)
	})

	mux.HandleFunc("/api/transfer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		var payload struct {
			DeviceID string `json:"device_id"`
			Play     bool   `json:"play"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if payload.DeviceID == "" {
			writeJSONError(w, http.StatusBadRequest, "missing device_id")
			return
		}
		body := map[string]any{
			"device_ids": []string{payload.DeviceID},
			"play":       payload.Play,
		}
		status, respBody, err := spotify.Do(r.Context(), http.MethodPut, "/me/player", nil, body)
		writeSpotifyResponseWithCache(w, status, respBody, err, playback)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		spotify := spotifyManager.Client()
		if spotify == nil {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		queryStr := r.URL.Query().Get("q")
		if queryStr == "" {
			queryStr = r.URL.Query().Get("query")
		}
		if queryStr == "" {
			writeJSONError(w, http.StatusBadRequest, "missing query")
			return
		}
		limit := r.URL.Query().Get("limit")
		if limit == "" {
			limit = "12"
		}
		query := url.Values{}
		query.Set("q", queryStr)
		query.Set("type", "track")
		query.Set("limit", limit)

		status, body, err := spotify.Do(r.Context(), http.MethodGet, "/search", query, nil)
		writeSpotifyResponse(w, status, body, err)
	})
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intString(v int) string {
	return strconv.Itoa(v)
}

func writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	if status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if status <= 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

func writeSpotifyResponse(w http.ResponseWriter, status int, body []byte, err error) {
	if err != nil {
		log.Printf("spotify api request failed: status=%d err=%v", status, err)
		if isSpotifyReloginError(err) {
			writeJSONError(w, http.StatusUnauthorized, "spotify connection expired. please re-login")
			return
		}
		if isSpotifyUnavailableError(err) {
			writeJSONError(w, http.StatusServiceUnavailable, spotifyNotConfiguredMessage)
			return
		}
		if status >= http.StatusBadRequest && len(body) > 0 {
			writeRawJSON(w, status, body)
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, body)
}

func writeSpotifyResponseWithCache(w http.ResponseWriter, status int, body []byte, err error, playback *PlaybackCache) {
	if err != nil && isNoActiveDevice(body, err) {
		if cached, ok := pausedCachedPlayback(playback); ok {
			writeRawJSON(w, http.StatusOK, cached)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeSpotifyResponse(w, status, body, err)
}

func pausedCachedPlayback(playback *PlaybackCache) ([]byte, bool) {
	cached, ok := playback.Get()
	if !ok {
		return nil, false
	}

	var payload map[string]any
	if err := json.Unmarshal(cached, &payload); err != nil {
		return cached, true
	}

	payload["is_playing"] = false
	normalized, err := json.Marshal(payload)
	if err != nil {
		return cached, true
	}
	return normalized, true
}

func isNoActiveDevice(body []byte, err error) bool {
	if err == nil {
		return false
	}
	var payload struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
			if strings.EqualFold(payload.Error.Reason, "NO_ACTIVE_DEVICE") {
				return true
			}
			if strings.Contains(strings.ToLower(payload.Error.Message), "no active device") {
				return true
			}
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no active device")
}

func isSpotifyUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "missing spotify_client_id") ||
		strings.Contains(message, "missing spotify_client_secret") ||
		strings.Contains(message, "missing spotify_refresh_token") ||
		strings.Contains(message, "missing access_token in refresh response") ||
		strings.Contains(message, "spotify client is nil")
}

func isSpotifyReloginError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "refresh token error:") || strings.Contains(message, "invalid_grant")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "code": status})
}
