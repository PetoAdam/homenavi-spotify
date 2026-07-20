package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const spotifyAPIBase = "https://api.spotify.com/v1"
const spotifyTokenURL = "https://accounts.spotify.com/api/token" // #nosec G101 -- URL, not a credential

type SpotifyClient struct {
	clientID     string
	clientSecret string
	refreshToken string
	setupPath    string

	httpClient *http.Client
	mu         sync.Mutex
	accessTok  string
	expiresAt  time.Time
}

type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	SetupPath    string
}

type SpotifyManager struct {
	mu                sync.RWMutex
	client            *SpotifyClient
	setupPath         string
	legacySecretsPath string
}

func NewSpotifyClientFromEnv() (*SpotifyClient, error) {
	config, err := loadSpotifyConfigFromSources(DefaultSetupPath(), selectSecretsPath())
	if err != nil {
		return nil, err
	}
	return NewSpotifyClient(config)
}

func NewSpotifyManagerFromEnv() (*SpotifyManager, error) {
	manager := &SpotifyManager{
		setupPath:         DefaultSetupPath(),
		legacySecretsPath: selectSecretsPath(),
	}
	err := manager.Reload()
	return manager, err
}

func (m *SpotifyManager) Reload() error {
	if m == nil {
		return errors.New("spotify manager is nil")
	}
	config, err := loadSpotifyConfigFromSources(m.setupPath, m.legacySecretsPath)
	if err != nil {
		m.mu.Lock()
		m.client = nil
		m.mu.Unlock()
		log.Printf("spotify manager reload failed: %v", err)
		return err
	}
	client, err := NewSpotifyClient(config)
	if err != nil {
		m.mu.Lock()
		m.client = nil
		m.mu.Unlock()
		log.Printf("spotify manager init failed: %v", err)
		return err
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	log.Printf("spotify manager reloaded")
	return nil
}

func (m *SpotifyManager) Client() *SpotifyClient {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

func NewSpotifyClient(config SpotifyConfig) (*SpotifyClient, error) {
	clientID := strings.TrimSpace(config.ClientID)
	clientSecret := strings.TrimSpace(config.ClientSecret)
	refreshToken := strings.TrimSpace(config.RefreshToken)
	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, errors.New("missing SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, or SPOTIFY_REFRESH_TOKEN")
	}

	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		setupPath:    strings.TrimSpace(config.SetupPath),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func loadSpotifyConfigFromSources(setupPath, legacySecretsPath string) (SpotifyConfig, error) {
	setupValues := loadSetupFromFile(setupPath)
	legacySecrets := loadSecretsFromFile(legacySecretsPath, "spotify")
	config := SpotifyConfig{
		ClientID:     strings.TrimSpace(getenv("SPOTIFY_CLIENT_ID", settingString(setupValues, "client_id"))),
		ClientSecret: strings.TrimSpace(getenv("SPOTIFY_CLIENT_SECRET", settingString(setupValues, "client_secret"))),
		RefreshToken: strings.TrimSpace(getenv("SPOTIFY_REFRESH_TOKEN", settingString(setupValues, "refresh_token"))),
		SetupPath:    strings.TrimSpace(setupPath),
	}
	if config.ClientID == "" {
		config.ClientID = strings.TrimSpace(legacySecrets["SPOTIFY_CLIENT_ID"])
	}
	if config.ClientSecret == "" {
		config.ClientSecret = strings.TrimSpace(legacySecrets["SPOTIFY_CLIENT_SECRET"])
	}
	if config.RefreshToken == "" {
		config.RefreshToken = strings.TrimSpace(legacySecrets["SPOTIFY_REFRESH_TOKEN"])
	}
	if config.ClientID == "" || config.ClientSecret == "" || config.RefreshToken == "" {
		return SpotifyConfig{}, errors.New("missing SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, or SPOTIFY_REFRESH_TOKEN")
	}
	return config, nil
}

func loadSetupFromFile(path string) map[string]any {
	if strings.TrimSpace(path) == "" {
		return map[string]any{}
	}
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path comes from env/default config
	if err != nil {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{}
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload
}

func loadSecretsFromFile(path, integrationID string) map[string]string {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(integrationID) == "" {
		return map[string]string{}
	}
	path = filepath.Clean(path)
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from env/default config
	if err != nil {
		return map[string]string{}
	}
	var byIntegration map[string]map[string]string
	if err := json.Unmarshal(data, &byIntegration); err == nil {
		return byIntegration[integrationID]
	}
	var flat map[string]string
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}
	return map[string]string{}
}

func selectSecretsPath() string {
	if path := strings.TrimSpace(os.Getenv("INTEGRATION_SECRETS_PATH")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("INTEGRATIONS_SECRETS_PATH")); path != "" {
		return path
	}
	return filepath.Join("config", "integration.secrets.json")
}

func (c *SpotifyClient) Do(ctx context.Context, method, path string, query url.Values, body any) (int, []byte, error) {
	if c == nil {
		return 0, nil, errors.New("spotify client is nil")
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return 0, nil, err
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint := spotifyAPIBase + path
	if len(query) > 0 {
		endpoint = endpoint + "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(payload))
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, data, fmt.Errorf("spotify api error: %s", strings.TrimSpace(string(data)))
	}
	return resp.StatusCode, data, nil
}

func (c *SpotifyClient) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessTok != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		tok := c.accessTok
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	if err := c.refreshAccessToken(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	tok := c.accessTok
	c.mu.Unlock()
	return tok, nil
}

func (c *SpotifyClient) refreshAccessToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(string(data))
		log.Printf("spotify access token refresh failed: status=%d body=%s", resp.StatusCode, message)
		return fmt.Errorf("refresh token error: %s", message)
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed.AccessToken == "" {
		return errors.New("missing access_token in refresh response")
	}
	if strings.TrimSpace(parsed.RefreshToken) != "" && strings.TrimSpace(parsed.RefreshToken) != strings.TrimSpace(c.refreshToken) {
		c.refreshToken = strings.TrimSpace(parsed.RefreshToken)
		_ = persistSpotifyRefreshToken(c.setupPath, c.refreshToken)
		log.Printf("spotify refresh token rotated and persisted")
	}
	c.accessTok = parsed.AccessToken
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 3600
	}
	c.expiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	_ = touchSpotifyTokenRefreshMetadata(c.setupPath)
	log.Printf("spotify access token refreshed; expires_in=%ds", parsed.ExpiresIn)
	return nil
}

func persistSpotifyRefreshToken(path, refreshToken string) error {
	store := NewSetupStore(path)
	now := time.Now().UTC()
	return store.Update(func(current map[string]any) (map[string]any, error) {
		next := cloneSettings(current)
		next["refresh_token"] = strings.TrimSpace(refreshToken)
		next["refresh_token_updated_at"] = now.Format(time.RFC3339)
		next["refresh_token_expires_at"] = now.Add(spotifyRefreshTokenLifetime).Format(time.RFC3339)
		next["token_last_refreshed_at"] = now.Format(time.RFC3339)
		return next, nil
	})
}

func touchSpotifyTokenRefreshMetadata(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	store := NewSetupStore(path)
	now := time.Now().UTC()
	return store.Update(func(current map[string]any) (map[string]any, error) {
		next := cloneSettings(current)
		next["token_last_refreshed_at"] = now.Format(time.RFC3339)
		return next, nil
	})
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
