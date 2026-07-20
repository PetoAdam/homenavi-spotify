package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SetupStore struct {
	path string
	mu   sync.Mutex
}

func NewSetupStore(path string) *SetupStore {
	return &SetupStore{path: path}
}

func DefaultSetupPath() string {
	if v := strings.TrimSpace(os.Getenv("INTEGRATION_SETUP_PATH")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("INTEGRATIONS_SETUP_PATH")); v != "" {
		return v
	}
	return filepath.Join("config", "integration.setup.json")
}

func (s *SetupStore) Get() (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *SetupStore) Set(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(values)
}

func (s *SetupStore) Update(mutator func(map[string]any) (map[string]any, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	next, err := mutator(current)
	if err != nil {
		return err
	}
	if next == nil {
		next = map[string]any{}
	}
	return s.saveUnlocked(next)
}

func (s *SetupStore) loadUnlocked() (map[string]any, error) {
	if strings.TrimSpace(s.path) == "" {
		return map[string]any{}, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{}, nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func (s *SetupStore) saveUnlocked(values map[string]any) error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
