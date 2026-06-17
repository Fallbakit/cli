package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// client talks to the dashboard's /api/dashboard/* and /api/cli/* routes using a
// CLI bearer token. The dashboard owns all management logic; the CLI is a thin
// front-end over it.
type client struct {
	dashboardURL string
	token        string
	http         *http.Client
}

func newClient(dashboardURL, token string) *client {
	return &client{
		dashboardURL: strings.TrimRight(dashboardURL, "/"),
		token:        token,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// apiError is the dashboard's consistent error envelope: {"error": "code"}.
type apiError struct {
	Status int
	Code   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("request failed (%d): %s", e.Status, e.Code)
}

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.dashboardURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.dashboardURL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusAccepted {
		// Used by device poll to signal authorization_pending.
		if out != nil && len(data) > 0 {
			_ = json.Unmarshal(data, out)
		}
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{Status: resp.StatusCode, Code: extractErrorCode(data, resp.StatusCode)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func extractErrorCode(data []byte, status int) string {
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Error != "" {
		return envelope.Error
	}
	if status == http.StatusUnauthorized {
		return "unauthorized"
	}
	if len(data) > 0 && len(data) < 200 {
		return strings.TrimSpace(string(data))
	}
	return "unexpected_error"
}

func isUnauthorized(err error) bool {
	var apiErr *apiError
	if asAPIError(err, &apiErr) {
		return apiErr.Status == http.StatusUnauthorized
	}
	return false
}

func asAPIError(err error, target **apiError) bool {
	for err != nil {
		if e, ok := err.(*apiError); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
