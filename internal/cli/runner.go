package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"
)

type runnerSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Active           bool   `json:"active"`
	RuntimeType      string `json:"runtimeType"`
	DeploymentTarget string `json:"deploymentTarget"`
	Status           string `json:"status"`
	LastSeenAt       string `json:"lastSeenAt"`
}

type diagnosticStep struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LatencyMs int    `json:"latencyMs"`
	Message   string `json:"message"`
}

type diagnosticResult struct {
	RunnerID string           `json:"runnerId"`
	Ready    bool             `json:"ready"`
	Status   string           `json:"status"`
	Steps    []diagnosticStep `json:"steps"`
}

var validRuntimes = []string{"ollama", "omlx", "vllm"}

func cmdRunner(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return runnerUsage()
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return cmdRunnerCreate(ctx, rest)
	case "list", "ls":
		return cmdRunnerList(ctx, rest)
	case "status":
		return cmdRunnerStatus(ctx, rest)
	case "up", "start":
		return cmdRunnerUp(ctx, rest)
	case "rotate":
		return cmdRunnerRotate(ctx, rest)
	case "rm", "delete":
		return cmdRunnerRemove(ctx, rest)
	case "-h", "--help", "help":
		return runnerUsage()
	default:
		return fmt.Errorf("unknown runner command %q (try `fallbakit runner help`)", sub)
	}
}

func runnerUsage() error {
	printInfo(`Manage local runners.

Usage: fallbakit runner <command>

Commands:
  create   Create a runner and (optionally) launch the agent
  list     List runners
  status   Show all runner statuses, or diagnose one: status <id>
  up       Launch the agent for a configured runner: up [<id>]
  rotate   Rotate a runner's API key: rotate <id>
  rm       Delete a runner: rm <id>`)
	return nil
}

func cmdRunnerCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("runner create", flag.ContinueOnError)
	name := fs.String("name", "", "runner name")
	runtime := fs.String("runtime", "", "runtime: ollama, omlx, or vllm")
	target := fs.String("target", "binary", "deployment target: binary or docker")
	localURL := fs.String("local-url", "", "local runtime base URL")
	launch := fs.Bool("launch", false, "launch the agent after creating the runner")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}

	runtimeType := strings.TrimSpace(*runtime)
	if runtimeType == "" {
		runtimeType = promptChoice("Select the local runtime:", validRuntimes, 0)
	}
	if !contains(validRuntimes, runtimeType) {
		return fmt.Errorf("invalid runtime %q (expected ollama, omlx, or vllm)", runtimeType)
	}
	runnerName := strings.TrimSpace(*name)
	if runnerName == "" {
		runnerName = prompt("Runner name", defaultRunnerName(runtimeType))
	}

	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	var created struct {
		Runner       runnerSummary `json:"runner"`
		RunnerID     string        `json:"runnerId"`
		RawKey       string        `json:"rawKey"`
		Instructions []string      `json:"instructions"`
	}
	body := map[string]string{"name": runnerName, "runtimeType": runtimeType, "deploymentTarget": *target}
	if err := c.do(ctx, "POST", "/api/dashboard/runners", body, &created); err != nil {
		return runnerError(err)
	}

	localBaseURL := strings.TrimSpace(*localURL)
	if localBaseURL == "" {
		localBaseURL = defaultLocalBaseURL(runtimeType)
	}
	rec := &RunnerRecord{
		RunnerID:     created.RunnerID,
		Name:         runnerName,
		APIKey:       created.RawKey,
		RuntimeType:  runtimeType,
		LocalBaseURL: localBaseURL,
	}
	if err := saveRunnerRecord(rec); err != nil {
		return err
	}

	printSuccess("Created runner %s (%s)", bold(runnerName), created.RunnerID)
	printInfo("%s", dim("Credentials saved to ~/.fallbakit/runners/"+created.RunnerID+".json"))
	printInfo("")
	printInfo("%s", bold("Runner environment:"))
	for _, line := range rec.envLines(cfg.resolvedAPIBaseURL()) {
		printInfo("  %s", line)
	}

	if *launch {
		printInfo("")
		return launchAgent(ctx, rec, cfg.resolvedAPIBaseURL())
	}
	printInfo("")
	printInfo("Start it now with: %s", bold("fallbakit runner up "+created.RunnerID))
	return nil
}

func cmdRunnerList(ctx context.Context, args []string) error {
	runners, err := fetchRunners(ctx)
	if err != nil {
		return err
	}
	renderRunnerTable(runners)
	return nil
}

func cmdRunnerStatus(ctx context.Context, args []string) error {
	if len(args) == 0 {
		// No id: show every runner's live status.
		runners, err := fetchRunners(ctx)
		if err != nil {
			return err
		}
		renderRunnerTable(runners)
		return nil
	}

	fs := flag.NewFlagSet("runner status", flag.ContinueOnError)
	watch := fs.Bool("watch", false, "re-run diagnostics every few seconds")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	runnerID := args[0]
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	for {
		var resp struct {
			Diagnostic diagnosticResult `json:"diagnostic"`
		}
		if err := c.do(ctx, "POST", "/api/dashboard/runners", map[string]string{"action": "test", "runnerId": runnerID}, &resp); err != nil {
			return runnerError(err)
		}
		renderDiagnostic(runnerID, resp.Diagnostic)
		if !*watch {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			printInfo("")
		}
	}
}

func cmdRunnerUp(ctx context.Context, args []string) error {
	cfg, _, err := requireAuth()
	if err != nil {
		return err
	}
	runnerID := ""
	if len(args) > 0 {
		runnerID = args[0]
	}
	rec, err := resolveRunnerRecord(runnerID)
	if err != nil {
		return err
	}
	return launchAgent(ctx, rec, cfg.resolvedAPIBaseURL())
}

func cmdRunnerRotate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fallbakit runner rotate <runner-id>")
	}
	runnerID := args[0]
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	var resp struct {
		RawKey string `json:"rawKey"`
	}
	if err := c.do(ctx, "POST", "/api/dashboard/runners", map[string]string{"action": "rotate", "runnerId": runnerID}, &resp); err != nil {
		return runnerError(err)
	}
	if rec, recErr := loadRunnerRecord(runnerID); recErr == nil {
		rec.APIKey = resp.RawKey
		_ = saveRunnerRecord(rec)
	}
	printSuccess("Rotated key for %s", runnerID)
	printInfo("%s", dim("Updated credentials saved locally. Restart the agent to use the new key."))
	return nil
}

func cmdRunnerRemove(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fallbakit runner rm <runner-id>")
	}
	runnerID := args[0]
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	if err := c.do(ctx, "DELETE", "/api/dashboard/runners?runnerId="+runnerID, nil, nil); err != nil {
		return runnerError(err)
	}
	printSuccess("Deleted runner %s", runnerID)
	return nil
}

func fetchRunners(ctx context.Context) ([]runnerSummary, error) {
	cfg, auth, err := requireAuth()
	if err != nil {
		return nil, err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	var resp struct {
		Runners []runnerSummary `json:"runners"`
	}
	if err := c.do(ctx, "GET", "/api/dashboard/runners", nil, &resp); err != nil {
		return nil, runnerError(err)
	}
	return resp.Runners, nil
}

func renderRunnerTable(runners []runnerSummary) {
	if len(runners) == 0 {
		printInfo("No runners yet. Create one with %s", bold("fallbakit runner create"))
		return
	}
	rows := make([][]string, 0, len(runners))
	for _, r := range runners {
		rows = append(rows, []string{r.ID, r.Name, r.RuntimeType, colorStatus(r.Status), lastSeen(r.LastSeenAt)})
	}
	table([]string{"ID", "NAME", "RUNTIME", "STATUS", "LAST SEEN"}, rows)
}

func renderDiagnostic(runnerID string, d diagnosticResult) {
	state := colorStatus(firstNonEmpty(d.Status, statusFromReady(d.Ready)))
	printInfo("%s %s  (%s)", bold("Runner"), runnerID, state)
	rows := make([][]string, 0, len(d.Steps))
	for _, step := range d.Steps {
		rows = append(rows, []string{stepMark(step.Status), step.Name, fmt.Sprintf("%dms", step.LatencyMs), step.Message})
	}
	table([]string{"", "CHECK", "LATENCY", "DETAIL"}, rows)
}

func colorStatus(status string) string {
	switch status {
	case "connected":
		return green(status)
	case "offline":
		return red(status)
	case "pending":
		return yellow(status)
	default:
		return status
	}
}

func stepMark(status string) string {
	if status == "passed" {
		return green("✓")
	}
	return red("✗")
}

func statusFromReady(ready bool) string {
	if ready {
		return "connected"
	}
	return "offline"
}

func lastSeen(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return value
}

func runnerError(err error) error {
	var apiErr *apiError
	if asAPIError(err, &apiErr) {
		switch apiErr.Code {
		case "runner_name_exists":
			return errors.New("a runner with that name already exists")
		case "runner_not_found":
			return errors.New("runner not found")
		case "runner_in_use":
			return errors.New("cannot delete a runner that has connected; deactivate it in the dashboard instead")
		case "database_not_configured":
			return errors.New("the platform database is not configured")
		case "unauthorized":
			return errors.New("not authorized — run `fallbakit login`")
		}
	}
	return err
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func defaultRunnerName(runtimeType string) string {
	switch runtimeType {
	case "vllm":
		return "Local vLLM runner"
	case "omlx":
		return "Local oMLX runner"
	default:
		return "Local Ollama runner"
	}
}

func defaultLocalBaseURL(runtimeType string) string {
	if runtimeType == "ollama" {
		return "http://localhost:11434"
	}
	return "http://localhost:8000"
}
