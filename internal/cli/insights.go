package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// insights.go implements the traffic-inspection commands (`requests` and
// `usage`). Both are exposed at the top level (`fallbakit requests`,
// `fallbakit usage`) and, scoped to an application, under `fallbakit app`.
// They read the dashboard's `/api/dashboard/requests` and `/api/dashboard/usage`
// routes with a `cli_` token.

type requestLogItem struct {
	RequestID        string  `json:"requestId"`
	ApplicationID    string  `json:"applicationId"`
	Timestamp        string  `json:"timestamp"`
	RoutedTo         string  `json:"routedTo"`
	RequestedModel   string  `json:"requestedModel"`
	ActualModel      string  `json:"actualModel"`
	Provider         string  `json:"provider"`
	LatencyMs        int     `json:"latencyMs"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	EstimatedCostUsd float64 `json:"estimatedCostUsd"`
	Status           string  `json:"status"`
	ErrorCode        string  `json:"errorCode"`
}

type requestsResponse struct {
	Requests     []requestLogItem `json:"requests"`
	RequestTotal int              `json:"requestTotal"`
	Range        string           `json:"range"`
}

type overviewMetrics struct {
	TotalRequests       int     `json:"totalRequests"`
	LocalRequests       int     `json:"localRequests"`
	CloudRequests       int     `json:"cloudRequests"`
	ErrorRequests       int     `json:"errorRequests"`
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	EstimatedCostUsd    float64 `json:"estimatedCostUsd"`
	EstimatedSavingsUsd float64 `json:"estimatedSavingsUsd"`
	AvgLatencyMs        float64 `json:"avgLatencyMs"`
}

type usagePoint struct {
	Label      string  `json:"label"`
	Requests   int     `json:"requests"`
	Local      int     `json:"local"`
	Cloud      int     `json:"cloud"`
	CostUsd    float64 `json:"costUsd"`
	SavingsUsd float64 `json:"savingsUsd"`
}

type usageResponse struct {
	Metrics overviewMetrics `json:"metrics"`
	Usage   []usagePoint    `json:"usage"`
	Range   string          `json:"range"`
}

// cmdRequests renders recent request logs for an application.
func cmdRequests(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("requests", flag.ContinueOnError)
	app := fs.String("app", "default", "application id")
	limit := fs.Int("limit", 20, "maximum number of requests to show")
	rang := fs.String("range", "24h", "time range: 24h, 7d, or 30d")
	asJSON := fs.Bool("json", false, "print raw JSON instead of a table")
	watch := fs.Bool("watch", false, "refresh every few seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateRange(*rang); err != nil {
		return err
	}
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	path := fmt.Sprintf("/api/dashboard/requests?applicationId=%s&page=1&range=%s",
		url.QueryEscape(*app), url.QueryEscape(*rang))
	for {
		var resp requestsResponse
		if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
			return appError(err)
		}
		items := resp.Requests
		if *limit > 0 && len(items) > *limit {
			items = items[:*limit]
		}
		if *asJSON {
			if err := printJSON(items); err != nil {
				return err
			}
		} else {
			renderRequests(items, *app, *rang)
		}
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

// cmdUsage renders aggregated usage metrics for an application.
func cmdUsage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	app := fs.String("app", "default", "application id")
	rang := fs.String("range", "24h", "time range: 24h, 7d, or 30d")
	asJSON := fs.Bool("json", false, "print raw JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateRange(*rang); err != nil {
		return err
	}
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	path := fmt.Sprintf("/api/dashboard/usage?applicationId=%s&range=%s",
		url.QueryEscape(*app), url.QueryEscape(*rang))
	var resp usageResponse
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return appError(err)
	}
	if *asJSON {
		return printJSON(resp)
	}
	renderUsage(*app, *rang, resp.Metrics, resp.Usage)
	return nil
}

// withApp prepends `--app <id>` when an app-scoped command receives a positional
// id, so the app-scoped forms reuse the same flag parser as the top-level ones.
func withApp(args []string) []string {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return append([]string{"--app", args[0]}, args[1:]...)
	}
	return args
}

func validateRange(r string) error {
	switch r {
	case "24h", "7d", "30d":
		return nil
	default:
		return fmt.Errorf("invalid range %q (expected 24h, 7d, or 30d)", r)
	}
}

func renderRequests(items []requestLogItem, app, rang string) {
	if len(items) == 0 {
		printInfo("No requests for %s in the last %s.", bold(app), rang)
		return
	}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			shortTime(it.Timestamp),
			routeLabel(it.RoutedTo),
			modelLabel(it),
			requestStatus(it.Status),
			fmt.Sprintf("%dms", it.LatencyMs),
			fmt.Sprintf("%d/%d", it.PromptTokens, it.CompletionTokens),
			fmt.Sprintf("$%.4f", it.EstimatedCostUsd),
		})
	}
	table([]string{"TIME", "ROUTE", "MODEL", "STATUS", "LATENCY", "TOK in/out", "COST"}, rows)

	for _, it := range items {
		if it.RoutedTo == "cloud" {
			printInfo("")
			printNote("Embeddings routed to cloud fallback use a different model than your local runtime; those vectors are not comparable to local embeddings. Send embeddings with localModelOnly to disable fallback for indexing jobs.")
			break
		}
	}
}

func renderUsage(app, rang string, m overviewMetrics, points []usagePoint) {
	printInfo("%s  %s", bold("Usage for "+app), dim("(last "+rang+")"))
	printInfo("")
	summary := [][]string{
		{"Requests", fmt.Sprintf("%d", m.TotalRequests)},
		{"Local", green(fmt.Sprintf("%d", m.LocalRequests))},
		{"Cloud", yellow(fmt.Sprintf("%d", m.CloudRequests))},
		{"Errors", fmt.Sprintf("%d", m.ErrorRequests)},
		{"Tokens in/out", fmt.Sprintf("%d / %d", m.PromptTokens, m.CompletionTokens)},
		{"Avg latency", fmt.Sprintf("%.0fms", m.AvgLatencyMs)},
		{"Cloud spend", fmt.Sprintf("$%.2f", m.EstimatedCostUsd)},
		{"Est. savings", green(fmt.Sprintf("$%.2f", m.EstimatedSavingsUsd))},
	}
	table([]string{"METRIC", "VALUE"}, summary)
	if len(points) > 0 {
		printInfo("")
		rows := make([][]string, 0, len(points))
		for _, p := range points {
			rows = append(rows, []string{
				p.Label,
				fmt.Sprintf("%d", p.Requests),
				fmt.Sprintf("%d", p.Local),
				fmt.Sprintf("%d", p.Cloud),
				fmt.Sprintf("$%.2f", p.CostUsd),
				fmt.Sprintf("$%.2f", p.SavingsUsd),
			})
		}
		table([]string{"WHEN", "REQUESTS", "LOCAL", "CLOUD", "COST", "SAVINGS"}, rows)
	}
}

func modelLabel(it requestLogItem) string {
	if it.ActualModel != "" && it.ActualModel != it.RequestedModel {
		return it.RequestedModel + "→" + it.ActualModel
	}
	return firstNonEmpty(it.RequestedModel, "—")
}

func routeLabel(routed string) string {
	switch routed {
	case "local":
		return green("local")
	case "cloud":
		return yellow("cloud")
	default:
		return dim(firstNonEmpty(routed, "none"))
	}
}

func requestStatus(status string) string {
	if status == "success" {
		return green("success")
	}
	return red(firstNonEmpty(status, "error"))
}

func shortTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("01-02 15:04:05")
	}
	return value
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
