package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type applicationSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Active          bool   `json:"active"`
	AllowEverywhere bool   `json:"allowEverywhere"`
}

func cmdApp(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return appUsage()
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "init":
		return cmdAppInit(ctx, rest)
	case "list", "ls":
		return cmdAppList(ctx, rest)
	case "enable":
		return cmdAppSetActive(ctx, rest, true)
	case "disable":
		return cmdAppSetActive(ctx, rest, false)
	case "requests":
		return cmdRequests(ctx, withApp(rest))
	case "usage":
		return cmdUsage(ctx, withApp(rest))
	case "key":
		return cmdAppKey(ctx, rest)
	case "-h", "--help", "help":
		return appUsage()
	default:
		return fmt.Errorf("unknown app command %q (try `fallbakit app help`)", sub)
	}
}

func appUsage() error {
	printInfo(`Manage applications, their traffic, and their API keys.

Usage: fallbakit app <command>

Commands:
  init             Create an application + API key for the current project and scaffold it
  list             List applications
  enable <id>      Activate an application so its keys can route traffic
  disable <id>     Deactivate an application
  requests [<id>]  Show recent requests:  requests [<id>] [--limit N] [--range 24h|7d|30d] [--json] [--watch]
  usage [<id>]     Show usage totals:      usage [<id>] [--range 24h|7d|30d] [--json]
  key create       Create an API key:      key create [--app <id>] [--name <name>]
  key rm <id>      Revoke an API key:       key rm <id> [--app <id>]`)
	return nil
}

// cmdAppSetActive toggles an application's active flag via the PATCH route.
func cmdAppSetActive(ctx context.Context, args []string, active bool) error {
	verb := "enable"
	if !active {
		verb = "disable"
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: fallbakit app %s <application-id>", verb)
	}
	appID := args[0]
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	var resp struct {
		Application applicationSummary `json:"application"`
	}
	body := map[string]any{"applicationId": appID, "active": active}
	if err := c.do(ctx, "PATCH", "/api/dashboard/applications", body, &resp); err != nil {
		return appError(err)
	}
	if active {
		printSuccess("Enabled application %s", bold(appID))
	} else {
		printSuccess("Disabled application %s", bold(appID))
	}
	return nil
}

func cmdAppInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("app init", flag.ContinueOnError)
	name := fs.String("name", "", "application name")
	noFiles := fs.Bool("no-files", false, "create the application and key but do not write project files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	project := detectProject(cwd)
	appName := strings.TrimSpace(*name)
	if appName == "" {
		appName = prompt("Application name", filepath.Base(cwd))
	}

	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	appID, err := resolveApplication(ctx, c, appName)
	if err != nil {
		return err
	}

	var keyResp struct {
		APIKey struct {
			Name string `json:"name"`
		} `json:"apiKey"`
		RawKey string `json:"rawKey"`
	}
	keyName := fmt.Sprintf("%s cli key", project.Kind)
	if err := c.do(ctx, "POST", "/api/dashboard/api-keys", map[string]string{"applicationId": appID, "name": keyName}, &keyResp); err != nil {
		return appError(err)
	}
	printSuccess("Application %s ready (id %s)", bold(appName), appID)

	if *noFiles || project.Kind == "unknown" {
		if project.Kind == "unknown" && !*noFiles {
			printInfo("%s", dim("No package.json or pyproject.toml found — skipping file scaffolding."))
		}
		printInfo("")
		printInfo("%s", bold("API key (shown once):"))
		printInfo("  %s", keyResp.RawKey)
		printInfo("  %s", dim("Base URL: "+cfg.resolvedAPIBaseURL()))
		return nil
	}

	written, err := scaffoldProject(cwd, project, keyResp.RawKey, cfg.resolvedAPIBaseURL())
	if err != nil {
		return err
	}
	printInfo("")
	for _, f := range written {
		printSuccess("Wrote %s", f)
	}
	printInfo("")
	printInfo("Next: %s", bold(project.RunHint))
	return nil
}

func cmdAppList(ctx context.Context, args []string) error {
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)
	var resp struct {
		Applications []applicationSummary `json:"applications"`
	}
	if err := c.do(ctx, "GET", "/api/dashboard/applications", nil, &resp); err != nil {
		return appError(err)
	}
	if len(resp.Applications) == 0 {
		printInfo("No applications yet.")
		return nil
	}
	rows := make([][]string, 0, len(resp.Applications))
	for _, app := range resp.Applications {
		rows = append(rows, []string{app.ID, app.Name, activeLabel(app.Active)})
	}
	table([]string{"ID", "NAME", "ACTIVE"}, rows)
	return nil
}

func cmdAppKey(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fallbakit app key <create|rm>")
	}
	sub, rest := args[0], args[1:]
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	c := newClient(cfg.resolvedDashboardURL(), auth.Token)

	switch sub {
	case "create":
		fs := flag.NewFlagSet("app key create", flag.ContinueOnError)
		appID := fs.String("app", "default", "application id")
		keyName := fs.String("name", "cli key", "key name")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		var resp struct {
			RawKey string `json:"rawKey"`
		}
		if err := c.do(ctx, "POST", "/api/dashboard/api-keys", map[string]string{"applicationId": *appID, "name": *keyName}, &resp); err != nil {
			return appError(err)
		}
		printSuccess("Created API key for application %s", *appID)
		printInfo("%s", bold("Key (shown once):"))
		printInfo("  %s", resp.RawKey)
		return nil
	case "rm", "delete":
		fs := flag.NewFlagSet("app key rm", flag.ContinueOnError)
		appID := fs.String("app", "default", "application id")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		ids := fs.Args()
		if len(ids) == 0 {
			return errors.New("usage: fallbakit app key rm <key-id> [--app <id>]")
		}
		path := fmt.Sprintf("/api/dashboard/api-keys?id=%s&applicationId=%s", ids[0], *appID)
		if err := c.do(ctx, "DELETE", path, nil, nil); err != nil {
			return appError(err)
		}
		printSuccess("Revoked API key %s", ids[0])
		return nil
	default:
		return fmt.Errorf("unknown key command %q", sub)
	}
}

// resolveApplication creates the named application, reusing the existing one when
// the name is already taken.
func resolveApplication(ctx context.Context, c *client, name string) (string, error) {
	var created struct {
		Application applicationSummary `json:"application"`
	}
	err := c.do(ctx, "POST", "/api/dashboard/applications", map[string]any{"name": name, "allowEverywhere": true}, &created)
	if err == nil {
		return created.Application.ID, nil
	}
	var apiErr *apiError
	if asAPIError(err, &apiErr) && apiErr.Code == "application_already_exists" {
		// Find the existing application by name.
		var list struct {
			Applications []applicationSummary `json:"applications"`
		}
		if listErr := c.do(ctx, "GET", "/api/dashboard/applications", nil, &list); listErr != nil {
			return "", appError(listErr)
		}
		for _, app := range list.Applications {
			if strings.EqualFold(app.Name, name) {
				return app.ID, nil
			}
		}
	}
	return "", appError(err)
}

func appError(err error) error {
	var apiErr *apiError
	if asAPIError(err, &apiErr) {
		switch apiErr.Code {
		case "application_not_found":
			return errors.New("application not found")
		case "database_not_configured":
			return errors.New("the platform database is not configured")
		case "unauthorized":
			return errors.New("not authorized — run `fallbakit login`")
		}
	}
	return err
}

func activeLabel(active bool) string {
	if active {
		return green("yes")
	}
	return dim("no")
}
