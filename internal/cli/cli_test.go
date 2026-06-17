package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProject(t *testing.T) {
	dir := t.TempDir()
	if got := detectProject(dir).Kind; got != "unknown" {
		t.Fatalf("empty dir: got %q, want unknown", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectProject(dir).Kind; got != "node" {
		t.Fatalf("node dir: got %q, want node", got)
	}

	pyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pyDir, "pyproject.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectProject(pyDir).Kind; got != "python" {
		t.Fatalf("python dir: got %q, want python", got)
	}
}

func TestMergeEnvPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=3000\nFALLBAKIT_API_KEY=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mergeEnv(path, map[string]string{"FALLBAKIT_API_KEY": "or_new", "FALLBAKIT_BASE_URL": "http://x"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "PORT=3000") {
		t.Errorf("dropped unrelated key: %q", content)
	}
	if !strings.Contains(content, "FALLBAKIT_API_KEY=or_new") {
		t.Errorf("did not update key: %q", content)
	}
	if strings.Contains(content, "old") {
		t.Errorf("kept stale value: %q", content)
	}
	if !strings.Contains(content, "FALLBAKIT_BASE_URL=http://x") {
		t.Errorf("did not append new key: %q", content)
	}
}

func TestConfigURLPrecedence(t *testing.T) {
	t.Setenv("FALLBAKIT_DASHBOARD_URL", "")
	cfg := &Config{}
	if got := cfg.resolvedDashboardURL(); got != defaultDashboardURL {
		t.Errorf("default: got %q", got)
	}
	cfg.DashboardURL = "https://configured.example/"
	if got := cfg.resolvedDashboardURL(); got != "https://configured.example" {
		t.Errorf("config value: got %q", got)
	}
	t.Setenv("FALLBAKIT_DASHBOARD_URL", "https://env.example/")
	if got := cfg.resolvedDashboardURL(); got != "https://env.example" {
		t.Errorf("env override: got %q", got)
	}
}

func TestEnsureGitignoreAddsEntryOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	for i := 0; i < 2; i++ {
		if err := ensureGitignore(path, ".env"); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	if n := strings.Count(string(data), ".env"); n != 1 {
		t.Errorf("expected .env once, got %d in %q", n, string(data))
	}
}
