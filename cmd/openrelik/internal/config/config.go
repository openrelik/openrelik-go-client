package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openrelik/openrelik-go-client"
)

const (
	configDir        = ".openrelik"
	settingsFile     = "settings.json"
	authCredsFile    = "auth_creds.json"
	workersCacheFile = "workers_cache.json"
	dirPerm          = 0700
	filePerm         = 0600
)

type Settings struct {
	ServerURL string `json:"server_url"`
}

type Credentials struct {
	APIKey string `json:"api_key"`
}

func (c Credentials) String() string {
	return "********"
}

var baseDir string

// SetBaseDir sets the base directory for configuration files (used for testing).
func SetBaseDir(dir string) {
	baseDir = dir
}

func GetConfigDir() (string, error) {
	if baseDir != "" {
		return filepath.Join(baseDir, configDir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	// Consider using os.UserConfigDir() in the future for better OS integration.
	return filepath.Join(home, configDir), nil
}

func EnsureConfigDir() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", fmt.Errorf("could not create config directory: %w", err)
	}
	return dir, nil
}

func LoadSettings() (*Settings, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, settingsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read settings file %s: %w", path, err)
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings file %s: %w", path, err)
	}
	return &s, nil
}

func SaveSettings(s *Settings) error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}
	path := filepath.Join(dir, settingsFile)
	return saveAtomic(path, data)
}

func LoadCredentials() (*Credentials, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, authCredsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file %s: %w", path, err)
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials file %s: %w", path, err)
	}
	return &c, nil
}

func SaveCredentials(c *Credentials) error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	path := filepath.Join(dir, authCredsFile)
	return saveAtomic(path, data)
}

func LoadWorkersCache() ([]openrelik.Worker, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, workersCacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workers cache file %s: %w", path, err)
	}
	var w []openrelik.Worker
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workers cache file %s: %w", path, err)
	}
	return w, nil
}

func SaveWorkersCache(w []openrelik.Worker) error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workers cache: %w", err)
	}
	path := filepath.Join(dir, workersCacheFile)
	return saveAtomic(path, data)
}

// saveAtomic writes data to a temporary file and then renames it to the target path
// to ensure the write is atomic.
func saveAtomic(path string, data []byte) error {
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, filePerm); err != nil {
		return fmt.Errorf("failed to write temporary file %s: %w", tmpFile, err)
	}
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile) // Best effort cleanup
		return fmt.Errorf("failed to rename %s to %s: %w", tmpFile, path, err)
	}
	return nil
}
