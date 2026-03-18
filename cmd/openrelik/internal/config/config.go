package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir      = ".openrelik"
	settingsFile   = "settings.json"
	authCredsFile  = "auth_creds.json"
	dirPerm        = 0700
	filePerm       = 0600
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
	data, err := os.ReadFile(filepath.Join(dir, settingsFile))
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
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
		return err
	}
	return saveAtomic(filepath.Join(dir, settingsFile), data)
}

func LoadCredentials() (*Credentials, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, authCredsFile))
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
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
		return err
	}
	return saveAtomic(filepath.Join(dir, authCredsFile), data)
}

// saveAtomic writes data to a temporary file and then renames it to the target path
// to ensure the write is atomic.
func saveAtomic(path string, data []byte) error {
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, filePerm); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile) // Best effort cleanup
		return err
	}
	return nil
}
