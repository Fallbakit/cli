package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

// recordedRequest captures what the fake dashboard last received so tests can
// assert how the CLI built the request.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Query  url.Values
	Body   map[string]any
}

func record(rec *recordedRequest, r *http.Request) {
	if rec == nil {
		return
	}
	rec.Method = r.Method
	rec.Path = r.URL.Path
	rec.Auth = r.Header.Get("Authorization")
	rec.Query = r.URL.Query()
	rec.Body = nil
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			rec.Body = m
		}
	}
}

// insightsServer stands in for the dashboard's requests/usage/applications routes.
func insightsServer(t *testing.T, rec *recordedRequest) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard/requests", func(w http.ResponseWriter, r *http.Request) {
		record(rec, r)
		writeJSONResp(w, 200, map[string]any{
			"range":        "24h",
			"requestTotal": 2,
			"requests": []map[string]any{
				{"requestId": "req_1", "timestamp": "2026-05-06T10:00:00Z", "routedTo": "local", "requestedModel": "llama3", "status": "success", "latencyMs": 120, "promptTokens": 10, "completionTokens": 20, "estimatedCostUsd": 0.0},
				{"requestId": "req_2", "timestamp": "2026-05-06T10:01:00Z", "routedTo": "cloud", "requestedModel": "gpt-4o", "status": "error", "errorCode": "upstream", "latencyMs": 300, "promptTokens": 5, "completionTokens": 0, "estimatedCostUsd": 0.01},
			},
		})
	})
	mux.HandleFunc("/api/dashboard/usage", func(w http.ResponseWriter, r *http.Request) {
		record(rec, r)
		writeJSONResp(w, 200, map[string]any{
			"range":   "7d",
			"metrics": map[string]any{"totalRequests": 12, "localRequests": 10, "cloudRequests": 2, "errorRequests": 1, "promptTokens": 100, "completionTokens": 200, "estimatedCostUsd": 0.6, "estimatedSavingsUsd": 2.1, "avgLatencyMs": 420},
			"usage":   []map[string]any{{"label": "May 1", "requests": 4, "local": 3, "cloud": 1, "costUsd": 0.2, "savingsUsd": 0.7}},
		})
	})
	mux.HandleFunc("/api/dashboard/applications", func(w http.ResponseWriter, r *http.Request) {
		record(rec, r)
		active := false
		if rec != nil && rec.Body != nil {
			if v, ok := rec.Body["active"].(bool); ok {
				active = v
			}
		}
		writeJSONResp(w, 200, map[string]any{"application": map[string]any{"id": "app_1", "name": "App", "active": active}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withFakeAuth points the CLI at srv using env-based token + dashboard URL so the
// real command path (requireAuth → client) runs without on-disk credentials.
func withFakeAuth(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("FALLBAKIT_DASHBOARD_URL", srv.URL)
	t.Setenv("FALLBAKIT_TOKEN", "cli_test_token")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	data, _ := io.ReadAll(r)
	return string(data)
}

func TestWithApp(t *testing.T) {
	got := withApp([]string{"app_1", "--limit", "5"})
	want := []string{"--app", "app_1", "--limit", "5"}
	if len(got) != len(want) {
		t.Fatalf("withApp len = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("withApp[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// A leading flag is left untouched.
	if out := withApp([]string{"--app", "x"}); out[0] != "--app" {
		t.Fatalf("withApp mangled flags: %v", out)
	}
}

func TestValidateRange(t *testing.T) {
	for _, ok := range []string{"24h", "7d", "30d"} {
		if err := validateRange(ok); err != nil {
			t.Errorf("validateRange(%q) = %v", ok, err)
		}
	}
	if err := validateRange("1y"); err == nil {
		t.Error("validateRange(1y) should fail")
	}
}

func TestCmdRequestsBuildsQuery(t *testing.T) {
	var rec recordedRequest
	srv := insightsServer(t, &rec)
	withFakeAuth(t, srv)

	out := captureStdout(t, func() {
		if err := cmdRequests(context.Background(), []string{"--app", "billing", "--range", "7d"}); err != nil {
			t.Fatalf("cmdRequests: %v", err)
		}
	})

	if rec.Auth != "Bearer cli_test_token" {
		t.Errorf("auth header = %q", rec.Auth)
	}
	if rec.Query.Get("applicationId") != "billing" {
		t.Errorf("applicationId = %q", rec.Query.Get("applicationId"))
	}
	if rec.Query.Get("range") != "7d" {
		t.Errorf("range = %q", rec.Query.Get("range"))
	}
	if rec.Query.Get("page") != "1" {
		t.Errorf("page = %q", rec.Query.Get("page"))
	}
	if out == "" {
		t.Error("expected table output")
	}
}

func TestCmdRequestsJSONHonorsLimit(t *testing.T) {
	var rec recordedRequest
	srv := insightsServer(t, &rec)
	withFakeAuth(t, srv)

	out := captureStdout(t, func() {
		if err := cmdRequests(context.Background(), []string{"--limit", "1", "--json"}); err != nil {
			t.Fatalf("cmdRequests: %v", err)
		}
	})

	var items []requestLogItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after --limit 1, got %d", len(items))
	}
	if items[0].RequestID != "req_1" {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
}

func TestCmdUsageRendersAndJSON(t *testing.T) {
	var rec recordedRequest
	srv := insightsServer(t, &rec)
	withFakeAuth(t, srv)

	out := captureStdout(t, func() {
		if err := cmdUsage(context.Background(), []string{"--range", "7d", "--json"}); err != nil {
			t.Fatalf("cmdUsage: %v", err)
		}
	})
	if rec.Query.Get("range") != "7d" {
		t.Errorf("range = %q", rec.Query.Get("range"))
	}
	var resp usageResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if resp.Metrics.TotalRequests != 12 || len(resp.Usage) != 1 {
		t.Fatalf("unexpected usage payload: %+v", resp)
	}
}

func TestCmdAppEnableDisableSendsActive(t *testing.T) {
	cases := []struct {
		sub  string
		want bool
	}{
		{"enable", true},
		{"disable", false},
	}
	for _, tc := range cases {
		t.Run(tc.sub, func(t *testing.T) {
			var rec recordedRequest
			srv := insightsServer(t, &rec)
			withFakeAuth(t, srv)

			_ = captureStdout(t, func() {
				if err := cmdApp(context.Background(), []string{tc.sub, "app_1"}); err != nil {
					t.Fatalf("%s: %v", tc.sub, err)
				}
			})

			if rec.Method != "PATCH" {
				t.Errorf("method = %q, want PATCH", rec.Method)
			}
			if rec.Body["applicationId"] != "app_1" {
				t.Errorf("applicationId = %v", rec.Body["applicationId"])
			}
			if active, _ := rec.Body["active"].(bool); active != tc.want {
				t.Errorf("active = %v, want %v", rec.Body["active"], tc.want)
			}
		})
	}
}

func TestCmdAppDisableRequiresID(t *testing.T) {
	if err := cmdAppSetActive(context.Background(), nil, false); err == nil {
		t.Fatal("expected an error when no application id is given")
	}
}

func TestAppRequestsPositionalIDMapsToApp(t *testing.T) {
	var rec recordedRequest
	srv := insightsServer(t, &rec)
	withFakeAuth(t, srv)

	_ = captureStdout(t, func() {
		if err := cmdApp(context.Background(), []string{"requests", "billing", "--limit", "5"}); err != nil {
			t.Fatalf("app requests: %v", err)
		}
	})
	if rec.Query.Get("applicationId") != "billing" {
		t.Errorf("applicationId = %q, want billing", rec.Query.Get("applicationId"))
	}
}
