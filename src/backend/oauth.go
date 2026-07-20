package backend

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const spotifyRefreshTokenLifetime = 180 * 24 * time.Hour

var spotifyOAuthScopes = []string{
	"user-read-playback-state",
	"user-modify-playback-state",
	"user-read-currently-playing",
}

type SpotifySetupAPI struct {
	Store   *SetupStore
	Admin   *AdminAuth
	Manager *SpotifyManager
}

func NewSpotifySetupAPI(store *SetupStore, admin *AdminAuth, manager *SpotifyManager) *SpotifySetupAPI {
	return &SpotifySetupAPI{Store: store, Admin: admin, Manager: manager}
}

func (s *SpotifySetupAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/setup", s.handleSetup)
}

func (s *SpotifySetupAPI) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		settings, err := s.Store.Get()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"settings": sanitizeSpotifySetup(settings)})
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if payload.Settings == nil {
		payload.Settings = map[string]any{}
	}
	if err := s.Store.Update(func(current map[string]any) (map[string]any, error) {
		if current == nil {
			current = map[string]any{}
		}
		next := cloneSettings(current)
		clientID := strings.TrimSpace(settingString(payload.Settings, "client_id"))
		clientSecret := strings.TrimSpace(settingString(payload.Settings, "client_secret"))

		clientIDChanged := clientID != strings.TrimSpace(settingString(current, "client_id"))
		clientSecretChanged := clientSecret != strings.TrimSpace(settingString(current, "client_secret"))

		next["client_id"] = clientID
		next["client_secret"] = clientSecret

		if clientIDChanged || clientSecretChanged {
			delete(next, "refresh_token")
			delete(next, "connected_at")
			delete(next, "refresh_token_updated_at")
			delete(next, "refresh_token_expires_at")
			delete(next, "token_last_refreshed_at")
		}
		return next, nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Manager != nil {
		_ = s.Manager.Reload()
	}
	settings, _ := s.Store.Get()
	log.Printf("spotify setup updated: client_id=%t client_secret=%t refresh_token=%t", strings.TrimSpace(settingString(settings, "client_id")) != "", strings.TrimSpace(settingString(settings, "client_secret")) != "", strings.TrimSpace(settingString(settings, "refresh_token")) != "")
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "settings": sanitizeSpotifySetup(settings)})
}

type SpotifyAuthAPI struct {
	Store   *SetupStore
	Admin   *AdminAuth
	Manager *SpotifyManager
	States  *SpotifyOAuthStateStore
	Client  *http.Client
}

func NewSpotifyAuthAPI(store *SetupStore, admin *AdminAuth, manager *SpotifyManager) *SpotifyAuthAPI {
	return &SpotifyAuthAPI{
		Store:   store,
		Admin:   admin,
		Manager: manager,
		States:  NewSpotifyOAuthStateStore(),
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *SpotifyAuthAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/auth/status", s.handleStatus)
	mux.HandleFunc("/api/admin/auth/login", s.handleLogin)
	mux.HandleFunc("/api/admin/auth/callback", s.handleCallback)
	mux.HandleFunc("/api/admin/auth/disconnect", s.handleDisconnect)
}

func (s *SpotifyAuthAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	settings, err := s.Store.Get()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": spotifyAuthStatus(settings)})
}

func (s *SpotifyAuthAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		RedirectURI string `json:"redirect_uri"`
		ReturnTo    string `json:"return_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	settings, err := s.Store.Get()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	clientID := strings.TrimSpace(settingString(settings, "client_id"))
	clientSecret := strings.TrimSpace(settingString(settings, "client_secret"))
	if clientID == "" || clientSecret == "" {
		writeJSONError(w, http.StatusBadRequest, "client_id and client_secret must be configured first")
		return
	}
	redirectURI := strings.TrimSpace(payload.RedirectURI)
	returnTo := strings.TrimSpace(payload.ReturnTo)
	if redirectURI == "" || returnTo == "" {
		writeJSONError(w, http.StatusBadRequest, "redirect_uri and return_to are required")
		return
	}
	log.Printf("spotify login requested: return_to=%s redirect_uri=%s", returnTo, redirectURI)
	state, err := s.States.Create(SpotifyOAuthState{
		RedirectURI: redirectURI,
		ReturnTo:    returnTo,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create auth state")
		return
	}
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", strings.Join(spotifyOAuthScopes, " "))
	query.Set("state", state)
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_url": "https://accounts.spotify.com/authorize?" + query.Encode(),
	})
}

func (s *SpotifyAuthAPI) handleCallback(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	stateToken := strings.TrimSpace(r.URL.Query().Get("state"))
	state, ok := s.States.Consume(stateToken)
	if !ok {
		log.Printf("spotify auth callback rejected: invalid state")
		writeJSONError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	if providerErr := strings.TrimSpace(r.URL.Query().Get("error")); providerErr != "" {
		log.Printf("spotify auth callback denied by provider: error=%s", providerErr)
		http.Redirect(w, r, withQuery(state.ReturnTo, "spotify", "auth_error"), http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		log.Printf("spotify auth callback missing code")
		http.Redirect(w, r, withQuery(state.ReturnTo, "spotify", "auth_error"), http.StatusFound)
		return
	}
	settings, err := s.Store.Get()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	clientID := strings.TrimSpace(settingString(settings, "client_id"))
	clientSecret := strings.TrimSpace(settingString(settings, "client_secret"))
	if clientID == "" || clientSecret == "" {
		writeJSONError(w, http.StatusBadRequest, "client_id and client_secret must be configured first")
		return
	}
	tokens, err := exchangeSpotifyCode(r.Context(), s.Client, clientID, clientSecret, code, state.RedirectURI)
	if err != nil {
		log.Printf("spotify auth exchange failed: %v", err)
		http.Redirect(w, r, withQuery(state.ReturnTo, "spotify", "auth_error"), http.StatusFound)
		return
	}
	now := time.Now().UTC()
	if err := s.Store.Update(func(current map[string]any) (map[string]any, error) {
		if current == nil {
			current = map[string]any{}
		}
		next := cloneSettings(current)
		next["client_id"] = clientID
		next["client_secret"] = clientSecret
		next["refresh_token"] = tokens.RefreshToken
		next["connected_at"] = now.Format(time.RFC3339)
		next["refresh_token_updated_at"] = now.Format(time.RFC3339)
		next["refresh_token_expires_at"] = now.Add(spotifyRefreshTokenLifetime).Format(time.RFC3339)
		next["token_last_refreshed_at"] = now.Format(time.RFC3339)
		if tokens.Scope != "" {
			next["scope"] = tokens.Scope
		}
		return next, nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Manager != nil {
		_ = s.Manager.Reload()
	}
	log.Printf("spotify account connected: scopes=%s", strings.TrimSpace(tokens.Scope))
	http.Redirect(w, r, withQuery(state.ReturnTo, "spotify", "connected"), http.StatusFound)
}

func (s *SpotifyAuthAPI) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.Store.Update(func(current map[string]any) (map[string]any, error) {
		if current == nil {
			current = map[string]any{}
		}
		next := cloneSettings(current)
		delete(next, "refresh_token")
		delete(next, "connected_at")
		delete(next, "refresh_token_updated_at")
		delete(next, "refresh_token_expires_at")
		delete(next, "token_last_refreshed_at")
		delete(next, "scope")
		return next, nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Manager != nil {
		_ = s.Manager.Reload()
	}
	log.Printf("spotify account disconnected")
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type SpotifyOAuthState struct {
	RedirectURI string
	ReturnTo    string
	CreatedAt   time.Time
}

type SpotifyOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]SpotifyOAuthState
}

func NewSpotifyOAuthStateStore() *SpotifyOAuthStateStore {
	return &SpotifyOAuthStateStore{states: map[string]SpotifyOAuthState{}}
}

func (s *SpotifyOAuthStateStore) Create(state SpotifyOAuthState) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	s.pruneLocked()
	s.states[token] = state
	return token, nil
}

func (s *SpotifyOAuthStateStore) Consume(token string) (SpotifyOAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[token]
	if ok {
		delete(s.states, token)
	}
	s.pruneLocked()
	return state, ok
}

func (s *SpotifyOAuthStateStore) pruneLocked() {
	cutoff := time.Now().UTC().Add(-15 * time.Minute)
	for key, state := range s.states {
		if state.CreatedAt.Before(cutoff) {
			delete(s.states, key)
		}
	}
}

type spotifyTokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func exchangeSpotifyCode(ctx context.Context, client *http.Client, clientID, clientSecret, code, redirectURI string) (*spotifyTokenExchangeResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("spotify auth exchange failed: %s", strings.TrimSpace(string(data)))
	}
	var parsed spotifyTokenExchangeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.RefreshToken) == "" {
		return nil, fmt.Errorf("spotify auth exchange missing refresh token")
	}
	return &parsed, nil
}

func sanitizeSpotifySetup(settings map[string]any) map[string]any {
	if settings == nil {
		settings = map[string]any{}
	}
	expiresAt := settingString(settings, "refresh_token_expires_at")
	return map[string]any{
		"client_id":               settingString(settings, "client_id"),
		"client_secret":           settingString(settings, "client_secret"),
		"connected":               strings.TrimSpace(settingString(settings, "refresh_token")) != "",
		"has_refresh_token":       strings.TrimSpace(settingString(settings, "refresh_token")) != "",
		"refresh_token_expires_at": expiresAt,
		"refresh_token_expires_in": refreshTokenExpiryLabel(expiresAt),
		"refresh_token_expired":    refreshTokenExpired(expiresAt),
		"connected_at":             settingString(settings, "connected_at"),
		"scope":                    settingString(settings, "scope"),
	}
}

func spotifyAuthStatus(settings map[string]any) map[string]any {
	if settings == nil {
		settings = map[string]any{}
	}
	hasRefreshToken := strings.TrimSpace(settingString(settings, "refresh_token")) != ""
	expiresAt := settingString(settings, "refresh_token_expires_at")
	return map[string]any{
		"connected":               hasRefreshToken,
		"has_client_id":           strings.TrimSpace(settingString(settings, "client_id")) != "",
		"has_client_secret":       strings.TrimSpace(settingString(settings, "client_secret")) != "",
		"has_refresh_token":       hasRefreshToken,
		"connected_at":            settingString(settings, "connected_at"),
		"refresh_token_expires_at": expiresAt,
		"refresh_token_expires_in":  refreshTokenExpiryLabel(expiresAt),
		"refresh_token_expired":     refreshTokenExpired(expiresAt),
		"token_last_refreshed_at":   settingString(settings, "token_last_refreshed_at"),
		"scope":                     settingString(settings, "scope"),
	}
}

func refreshTokenExpired(expiresAt string) bool {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt))
	if err != nil {
		return false
	}
	return time.Now().UTC().After(parsed)
}

func refreshTokenExpiryLabel(expiresAt string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt))
	if err != nil || parsed.IsZero() {
		return ""
	}
	remaining := time.Until(parsed)
	if remaining <= 0 {
		return "expired"
	}
	days := int(remaining.Hours()) / 24
	hours := int(remaining.Hours()) % 24
	minutes := int(remaining.Minutes()) % 60
	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", minutes)
}

func cloneSettings(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func settingString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func withQuery(rawURL, key, value string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
