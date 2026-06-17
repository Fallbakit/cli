package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type projectInfo struct {
	Kind        string // "node", "python", or "unknown"
	ExampleFile string
	RunHint     string
}

// detectProject inspects the directory for language markers.
func detectProject(dir string) projectInfo {
	if fileExists(filepath.Join(dir, "package.json")) {
		return projectInfo{Kind: "node", ExampleFile: "fallbakit-example.mjs", RunHint: "npm install @fallbakit/sdk && node fallbakit-example.mjs"}
	}
	if fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "requirements.txt")) {
		return projectInfo{Kind: "python", ExampleFile: "fallbakit_example.py", RunHint: "pip install fallbakit && python fallbakit_example.py"}
	}
	return projectInfo{Kind: "unknown"}
}

// scaffoldProject writes/merges .env, ensures .gitignore covers it, and drops an
// SDK example. It returns the relative paths written.
func scaffoldProject(dir string, project projectInfo, apiKey, apiBaseURL string) ([]string, error) {
	written := []string{}

	envPath := filepath.Join(dir, ".env")
	if err := mergeEnv(envPath, map[string]string{
		"FALLBAKIT_API_KEY":  apiKey,
		"FALLBAKIT_BASE_URL": apiBaseURL,
	}); err != nil {
		return nil, err
	}
	written = append(written, ".env")

	if err := ensureGitignore(filepath.Join(dir, ".gitignore"), ".env"); err == nil {
		written = append(written, ".gitignore")
	}

	examplePath := filepath.Join(dir, project.ExampleFile)
	if !fileExists(examplePath) {
		content := nodeExample
		if project.Kind == "python" {
			content = pythonExample
		}
		if err := os.WriteFile(examplePath, []byte(content), 0o644); err != nil {
			return nil, err
		}
		written = append(written, project.ExampleFile)
	}
	return written, nil
}

// mergeEnv updates the given keys in a .env file without disturbing other lines.
func mergeEnv(path string, values map[string]string) error {
	existing := map[string]bool{}
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			key := ""
			if eq := strings.Index(trimmed, "="); eq > 0 && !strings.HasPrefix(trimmed, "#") {
				key = strings.TrimSpace(trimmed[:eq])
			}
			if newValue, ok := values[key]; ok {
				lines = append(lines, fmt.Sprintf("%s=%s", key, newValue))
				existing[key] = true
				continue
			}
			lines = append(lines, line)
		}
		// Drop a trailing empty line so appends stay tidy.
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for key, value := range values {
		if !existing[key] {
			lines = append(lines, fmt.Sprintf("%s=%s", key, value))
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func ensureGitignore(path, entry string) error {
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
		return os.WriteFile(path, append(data, []byte("\n"+entry+"\n")...), 0o644)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(entry+"\n"), 0o644)
}

const nodeExample = `import { Fallbakit } from "@fallbakit/sdk";

const apiKey = process.env.FALLBAKIT_API_KEY;
if (!apiKey) {
  throw new Error("Set FALLBAKIT_API_KEY before running this example.");
}

const client = new Fallbakit({
  apiKey,
  baseURL: process.env.FALLBAKIT_BASE_URL ?? "http://localhost:8080"
});

const response = await client.chat.completions.create({
  model: process.env.FALLBAKIT_MODEL ?? "llama3.2",
  messages: [{ role: "user", content: "Write a tiny launch checklist." }]
});

console.log(response.choices[0].message.content);
`

const pythonExample = `import os

from fallbakit import Fallbakit

api_key = os.environ.get("FALLBAKIT_API_KEY")
if not api_key:
    raise RuntimeError("Set FALLBAKIT_API_KEY before running this example.")

client = Fallbakit(
    api_key=api_key,
    base_url=os.environ.get("FALLBAKIT_BASE_URL", "http://localhost:8080"),
)

response = client.chat.completions.create(
    model=os.environ.get("FALLBAKIT_MODEL", "llama3.2"),
    messages=[{"role": "user", "content": "Write a tiny launch checklist."}],
)

print(response["choices"][0]["message"]["content"])
`
