package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDashboardURL = "https://fallbakit.com"
	defaultAPIBaseURL   = "https://api.fallbakit.com"
)

// Config holds non-secret CLI settings persisted to ~/.fallbakit/config.json.
type Config struct {
	DashboardURL string `json:"dashboardUrl,omitempty"`
	APIBaseURL   string `json:"apiBaseUrl,omitempty"`
}

// Auth holds the signed-in credentials persisted to ~/.fallbakit/auth.json (0600).
type Auth struct {
	Token     string `json:"token"`
	UserID    string `json:"userId,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

// RunnerRecord remembers a configured runner so `runner up` can launch the agent.
type RunnerRecord struct {
	RunnerID     string `json:"runnerId"`
	Name         string `json:"name,omitempty"`
	APIKey       string `json:"apiKey"`
	RuntimeType  string `json:"runtimeType"`
	LocalBaseURL string `json:"localBaseUrl,omitempty"`
}

func configHome() (string, error) {
	if override := strings.TrimSpace(os.Getenv("FALLBAKIT_CONFIG_HOME")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fallbakit"), nil
}

func ensureConfigHome() (string, error) {
	dir, err := configHome()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// resolvedDashboardURL applies precedence: env > config.json > default.
func (c *Config) resolvedDashboardURL() string {
	if v := strings.TrimSpace(os.Getenv("FALLBAKIT_DASHBOARD_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if strings.TrimSpace(c.DashboardURL) != "" {
		return strings.TrimRight(c.DashboardURL, "/")
	}
	return defaultDashboardURL
}

func (c *Config) resolvedAPIBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("FALLBAKIT_API_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if strings.TrimSpace(c.APIBaseURL) != "" {
		return strings.TrimRight(c.APIBaseURL, "/")
	}
	return defaultAPIBaseURL
}

func loadConfig() (*Config, error) {
	dir, err := configHome()
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	return cfg, nil
}

func saveConfig(cfg *Config) error {
	dir, err := ensureConfigHome()
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "config.json"), cfg, 0o600)
}

func loadAuth() (*Auth, error) {
	if token := strings.TrimSpace(os.Getenv("FALLBAKIT_TOKEN")); token != "" {
		return &Auth{Token: token}, nil
	}
	dir, err := configHome()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	auth := &Auth{}
	if err := json.Unmarshal(data, auth); err != nil {
		return nil, fmt.Errorf("parse auth.json: %w", err)
	}
	if strings.TrimSpace(auth.Token) == "" {
		return nil, nil
	}
	return auth, nil
}

func saveAuth(auth *Auth) error {
	dir, err := ensureConfigHome()
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "auth.json"), auth, 0o600)
}

func clearAuth() error {
	dir, err := configHome()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "auth.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveRunnerRecord(rec *RunnerRecord) error {
	dir, err := ensureConfigHome()
	if err != nil {
		return err
	}
	runnersDir := filepath.Join(dir, "runners")
	if err := os.MkdirAll(runnersDir, 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(runnersDir, rec.RunnerID+".json"), rec, 0o600)
}

func loadRunnerRecord(runnerID string) (*RunnerRecord, error) {
	dir, err := configHome()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "runners", runnerID+".json"))
	if err != nil {
		return nil, err
	}
	rec := &RunnerRecord{}
	if err := json.Unmarshal(data, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func writeJSON(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, perm)
}
