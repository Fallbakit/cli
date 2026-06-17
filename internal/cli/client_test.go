package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeDashboard stands in for the Next.js dashboard so we can exercise the CLI's
// HTTP layer (device login, whoami, error-envelope parsing) without a real server.
func fakeDashboard(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	approved := false

	mux.HandleFunc("/api/cli/device/start", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{
			"deviceCode":      "dev_abc",
			"userCode":        "WDJB-MJHT",
			"verificationUrl": "http://example/dashboard/cli?code=WDJB-MJHT",
			"interval":        1,
			"expiresIn":       60,
		})
	})
	mux.HandleFunc("/api/cli/device/poll", func(w http.ResponseWriter, r *http.Request) {
		if !approved {
			approved = true // approve on the second observation
			writeJSONResp(w, http.StatusAccepted, map[string]any{"status": "authorization_pending"})
			return
		}
		writeJSONResp(w, 200, map[string]any{"status": "approved", "token": "cli_test_token"})
	})
	mux.HandleFunc("/api/dashboard/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cli_test_token" {
			writeJSONResp(w, 401, map[string]any{"error": "unauthorized"})
			return
		}
		body := map[string]any{"userId": "user_1", "accountId": "acct_1", "applicationId": "default"}
		body["entitlement"] = map[string]any{"plan": "free", "runnerLimit": 1}
		writeJSONResp(w, 200, body)
	})
	mux.HandleFunc("/api/dashboard/runners", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{"runners": []map[string]any{
			{"id": "runner_1", "name": "Mac", "runtimeType": "ollama", "status": "connected"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSONResp(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func TestDeviceLoginAndWhoami(t *testing.T) {
	srv := fakeDashboard(t)
	ctx := context.Background()

	token, err := deviceLogin(ctx, srv.URL, true)
	if err != nil {
		t.Fatalf("deviceLogin: %v", err)
	}
	if token != "cli_test_token" {
		t.Fatalf("token = %q", token)
	}

	who, err := fetchWhoami(ctx, srv.URL, token)
	if err != nil {
		t.Fatalf("fetchWhoami: %v", err)
	}
	if who.UserID != "user_1" || who.AccountID != "acct_1" {
		t.Fatalf("whoami = %+v", who)
	}
}

func TestWhoamiUnauthorized(t *testing.T) {
	srv := fakeDashboard(t)
	_, err := fetchWhoami(context.Background(), srv.URL, "cli_wrong")
	if err == nil || !isUnauthorized(err) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestClientParsesRunnerList(t *testing.T) {
	srv := fakeDashboard(t)
	c := newClient(srv.URL, "cli_test_token")
	var resp struct {
		Runners []runnerSummary `json:"runners"`
	}
	if err := c.do(context.Background(), "GET", "/api/dashboard/runners", nil, &resp); err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(resp.Runners) != 1 || resp.Runners[0].Status != "connected" {
		t.Fatalf("runners = %+v", resp.Runners)
	}
}
