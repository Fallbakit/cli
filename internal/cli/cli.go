// Package cli implements the Fallbakit developer CLI: a thin, dependency-free
// front-end over the dashboard's management API.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// Version is overridden at build time via -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

// Run dispatches a command and returns a process exit code.
func Run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		usage()
		return 0
	}
	cmd, rest := args[0], args[1:]

	var err error
	switch cmd {
	case "login":
		err = cmdLogin(ctx, rest)
	case "logout":
		err = cmdLogout(ctx, rest)
	case "whoami":
		err = cmdWhoami(ctx, rest)
	case "runner":
		err = cmdRunner(ctx, rest)
	case "app":
		err = cmdApp(ctx, rest)
	case "requests":
		err = cmdRequests(ctx, rest)
	case "usage":
		err = cmdUsage(ctx, rest)
	case "config":
		err = cmdConfig(ctx, rest)
	case "version", "--version", "-v":
		fmt.Printf("fallbakit %s\n", Version)
	case "help", "-h", "--help":
		usage()
	default:
		printErr("unknown command %q", cmd)
		usage()
		return 1
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		printErr("%s", err.Error())
		return 1
	}
	return 0
}

func cmdConfig(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fallbakit config <get|set|path>")
	}
	switch args[0] {
	case "path":
		dir, err := configHome()
		if err != nil {
			return err
		}
		printInfo("%s", dir)
		return nil
	case "get":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		printInfo("dashboardUrl  %s", cfg.resolvedDashboardURL())
		printInfo("apiBaseUrl    %s", cfg.resolvedAPIBaseURL())
		return nil
	case "set":
		if len(args) < 3 {
			return errors.New("usage: fallbakit config set <dashboardUrl|apiBaseUrl> <value>")
		}
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		key, value := args[1], strings.TrimRight(args[2], "/")
		switch key {
		case "dashboardUrl":
			cfg.DashboardURL = value
		case "apiBaseUrl":
			cfg.APIBaseURL = value
		default:
			return fmt.Errorf("unknown config key %q (expected dashboardUrl or apiBaseUrl)", key)
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
		printSuccess("Set %s = %s", key, value)
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func usage() {
	fmt.Print(`Fallbakit — local-first AI inference with cloud fallback.

Usage: fallbakit <command> [flags]

Authentication:
  login            Sign in (opens a browser; or use --token <cli_token>)
  logout           Remove stored credentials
  whoami           Show the signed-in user

Runners:
  runner create    Create a runner and launch the agent
  runner list      List runners
  runner status    Show all runner statuses, or diagnose one: status <id>
  runner up        Launch the agent for a configured runner
  runner rotate    Rotate a runner's API key
  runner rm        Delete a runner

Applications:
  app init         Set up an application + API key for the current project
  app list         List applications
  app enable       Activate an application:   app enable <id>
  app disable      Deactivate an application: app disable <id>
  app key          Create or revoke API keys

Insights:
  requests         Show recent requests:  requests [--app <id>] [--limit N] [--range 24h|7d|30d] [--json] [--watch]
  usage            Show usage totals:     usage [--app <id>] [--range 24h|7d|30d] [--json]

Other:
  config           View or change CLI settings
  version          Print the CLI version

Run 'fallbakit <command> --help' for command-specific flags.
`)
}
