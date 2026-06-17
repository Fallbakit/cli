package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type whoamiResponse struct {
	UserID        string `json:"userId"`
	AccountID     string `json:"accountId"`
	ApplicationID string `json:"applicationId"`
	IsAdmin       bool   `json:"isAdmin"`
	Entitlement   struct {
		Plan        string `json:"plan"`
		RunnerLimit int    `json:"runnerLimit"`
	} `json:"entitlement"`
}

func cmdLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	token := fs.String("token", "", "authenticate with a CLI token instead of the browser flow")
	dashboardURL := fs.String("dashboard-url", "", "dashboard URL to authenticate against")
	noBrowser := fs.Bool("no-browser", false, "do not open a browser automatically")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*dashboardURL) != "" {
		cfg.DashboardURL = strings.TrimRight(*dashboardURL, "/")
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}
	base := cfg.resolvedDashboardURL()

	var rawToken string
	if strings.TrimSpace(*token) != "" {
		rawToken = strings.TrimSpace(*token)
	} else {
		rawToken, err = deviceLogin(ctx, base, *noBrowser)
		if err != nil {
			return err
		}
	}

	// Validate the token and capture identity.
	who, err := fetchWhoami(ctx, base, rawToken)
	if err != nil {
		if isUnauthorized(err) {
			return errors.New("token was rejected by the dashboard")
		}
		return err
	}
	if err := saveAuth(&Auth{Token: rawToken, UserID: who.UserID, AccountID: who.AccountID}); err != nil {
		return err
	}
	printSuccess("Logged in as %s", bold(who.UserID))
	printInfo("%s", dim(fmt.Sprintf("account %s · plan %s", who.AccountID, who.Entitlement.Plan)))
	return nil
}

func cmdLogout(ctx context.Context, args []string) error {
	if err := clearAuth(); err != nil {
		return err
	}
	printSuccess("Logged out")
	return nil
}

func cmdWhoami(ctx context.Context, args []string) error {
	cfg, auth, err := requireAuth()
	if err != nil {
		return err
	}
	who, err := fetchWhoami(ctx, cfg.resolvedDashboardURL(), auth.Token)
	if err != nil {
		if isUnauthorized(err) {
			return errors.New("not logged in or token expired — run `fallbakit login`")
		}
		return err
	}
	printInfo("%s  %s", bold("User:"), who.UserID)
	printInfo("%s  %s", bold("Account:"), who.AccountID)
	printInfo("%s  %s (runner limit %d)", bold("Plan:"), who.Entitlement.Plan, who.Entitlement.RunnerLimit)
	if who.IsAdmin {
		printInfo("%s  %s", bold("Role:"), "admin")
	}
	return nil
}

func fetchWhoami(ctx context.Context, base, token string) (*whoamiResponse, error) {
	c := newClient(base, token)
	who := &whoamiResponse{}
	if err := c.do(ctx, "GET", "/api/dashboard/whoami", nil, who); err != nil {
		return nil, err
	}
	return who, nil
}

// deviceLogin runs the OAuth-style device-authorization flow against the dashboard.
func deviceLogin(ctx context.Context, base string, noBrowser bool) (string, error) {
	c := newClient(base, "")
	var start struct {
		DeviceCode      string `json:"deviceCode"`
		UserCode        string `json:"userCode"`
		VerificationURL string `json:"verificationUrl"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expiresIn"`
	}
	if err := c.do(ctx, "POST", "/api/cli/device/start", map[string]any{}, &start); err != nil {
		return "", fmt.Errorf("start login: %w", err)
	}

	printInfo("")
	printInfo("To finish signing in, open:\n  %s", bold(start.VerificationURL))
	printInfo("and confirm this code: %s", bold(start.UserCode))
	printInfo("")
	if !noBrowser {
		if err := openBrowser(start.VerificationURL); err != nil {
			printInfo("%s", dim("(could not open a browser automatically; use the link above)"))
		}
	}

	interval := time.Duration(start.Interval) * time.Second
	if interval <= 0 {
		interval = 4 * time.Second
	}
	deadline := time.Now().Add(time.Duration(maxInt(start.ExpiresIn, 60)) * time.Second)
	printInfo("%s", dim("Waiting for approval…"))
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		var poll struct {
			Status string `json:"status"`
			Token  string `json:"token"`
		}
		err := c.do(ctx, "POST", "/api/cli/device/poll", map[string]string{"deviceCode": start.DeviceCode}, &poll)
		if err != nil {
			var apiErr *apiError
			if asAPIError(err, &apiErr) && (apiErr.Code == "expired_token" || apiErr.Code == "invalid_device_code") {
				return "", errors.New("login request expired — run `fallbakit login` again")
			}
			return "", err
		}
		if poll.Status == "approved" && poll.Token != "" {
			return poll.Token, nil
		}
	}
	return "", errors.New("login timed out waiting for approval")
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// requireAuth loads config and credentials, erroring with a helpful message when
// the user is not logged in.
func requireAuth() (*Config, *Auth, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, err
	}
	auth, err := loadAuth()
	if err != nil {
		return nil, nil, err
	}
	if auth == nil {
		return nil, nil, errors.New("not logged in — run `fallbakit login`")
	}
	return cfg, auth, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
