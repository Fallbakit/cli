package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// envLines returns the agent environment for this runner record.
func (r *RunnerRecord) envLines(apiBaseURL string) []string {
	localBaseURL := r.LocalBaseURL
	if localBaseURL == "" {
		localBaseURL = defaultLocalBaseURL(r.RuntimeType)
	}
	return []string{
		"FALLBAKIT_RUNNER_ID=" + r.RunnerID,
		"FALLBAKIT_RUNNER_API_KEY=" + r.APIKey,
		"FALLBAKIT_BASE_URL=" + apiBaseURL,
		"FALLBAKIT_LOCAL_PROVIDER=" + r.RuntimeType,
		"FALLBAKIT_LOCAL_BASE_URL=" + localBaseURL,
	}
}

// launchAgent locates the fallbakit-agent binary and execs it with this runner's
// credentials. The agent owns the long-lived tunnel; the CLI just starts it.
func launchAgent(ctx context.Context, rec *RunnerRecord, apiBaseURL string) error {
	bin, err := findAgentBinary()
	if err != nil {
		return err
	}
	printInfo("Starting agent for %s using %s…", bold(rec.RunnerID), bin)
	printInfo("%s", dim("Press Ctrl-C to stop."))

	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(), rec.envLines(apiBaseURL)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil // interrupted by the user
		}
		return fmt.Errorf("agent exited: %w", err)
	}
	return nil
}

// findAgentBinary looks for fallbakit-agent next to the CLI, then on PATH, then in
// the CLI's config bin directory (where the installer places it).
func findAgentBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("FALLBAKIT_AGENT_BIN")); override != "" {
		return override, nil
	}
	candidates := []string{}
	if self, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(self), "fallbakit-agent"))
	}
	if dir, err := configHome(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "bin", "fallbakit-agent"))
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("fallbakit-agent"); err == nil {
		return path, nil
	}
	return "", errors.New("could not find the `fallbakit-agent` binary. Install it alongside the CLI or set FALLBAKIT_AGENT_BIN")
}

// resolveRunnerRecord returns the requested runner record, or the only one when no
// id is supplied.
func resolveRunnerRecord(runnerID string) (*RunnerRecord, error) {
	if runnerID != "" {
		rec, err := loadRunnerRecord(runnerID)
		if err != nil {
			return nil, fmt.Errorf("no saved credentials for %s — create it with `fallbakit runner create`", runnerID)
		}
		return rec, nil
	}
	records, err := listRunnerRecords()
	if err != nil {
		return nil, err
	}
	switch len(records) {
	case 0:
		return nil, errors.New("no configured runners — run `fallbakit runner create`")
	case 1:
		return records[0], nil
	default:
		return nil, errors.New("multiple runners configured — specify one: `fallbakit runner up <id>`")
	}
}

func listRunnerRecords() ([]*RunnerRecord, error) {
	dir, err := configHome()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "runners"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	records := make([]*RunnerRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if rec, err := loadRunnerRecord(id); err == nil {
			records = append(records, rec)
		}
	}
	return records, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
