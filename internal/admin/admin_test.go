package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubAdminPorter satisfies AdminPorter for tests.
type stubAdminPorter struct{ port int }

func (s stubAdminPorter) AdminPort() int { return s.port }

func testProject(t *testing.T, token string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, TokenFileName), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadTokenFailClosed(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadToken(dir); err == nil {
		t.Fatal("missing token file must fail closed")
	}
}

func TestNewClientReadsTokenAndPort(t *testing.T) {
	dir := testProject(t, "tok-abc")
	c, err := NewClient(dir, stubAdminPorter{port: 9999}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "tok-abc" || c.BaseURL != "http://127.0.0.1:9999" {
		t.Fatalf("client misconfigured: %+v", c)
	}
}

func TestCreateKeyAndList(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /admin/keys":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "id-1", "prefix": "lbt_live_pre", "key": "lbt_live_FULL",
				"warning": "shown once",
			})
		case http.MethodGet + " /admin/keys":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": "id-1", "name": "n", "prefix": "lbt_live_pre",
				"rate_limit_rps": 5, "budget_usd": nil, "revoked_at": nil,
				"created_at": time.Now().Format(time.RFC3339),
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := testProject(t, "secret")
	c, err := NewClient(dir, stubAdminPorter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Point the client at the stub server (loopback host string is intact).
	c.BaseURL = srv.URL

	res, err := c.CreateKey(context.Background(), CreateKeyRequest{Name: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full != "lbt_live_FULL" || res.ID != "id-1" {
		t.Fatalf("create result wrong: %+v", res)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("token not sent as bearer: %q", gotAuth)
	}

	keys, err := c.ListKeys(context.Background())
	if err != nil || len(keys) != 1 || keys[0].Prefix != "lbt_live_pre" {
		t.Fatalf("list failed: %v %+v", err, keys)
	}
}

func TestRevokeAndErrorPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/keys/id-1/revoke":
			w.WriteHeader(http.StatusOK)
		case "/admin/keys/bad/revoke":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such key"))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dir := testProject(t, "secret")
	c, _ := NewClient(dir, stubAdminPorter{}, nil)
	c.BaseURL = srv.URL

	if err := c.RevokeKey(context.Background(), "id-1"); err != nil {
		t.Fatalf("revoke should succeed: %v", err)
	}
	err := c.RevokeKey(context.Background(), "bad")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("health against 500 server should fail")
	}
}

func TestUsageQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"by": "model", "since": "2026-01-01T00:00:00Z",
			"points":  []map[string]any{{"group": "fake-chat", "requests": 2}},
			"note":    "estimates", "month_spend": 0.5,
		})
	}))
	defer srv.Close()

	dir := testProject(t, "secret")
	c, _ := NewClient(dir, stubAdminPorter{}, nil)
	c.BaseURL = srv.URL
	res, err := c.Usage(context.Background(), "model", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 1 || res.Points[0].Group != "fake-chat" {
		t.Fatalf("usage decode wrong: %+v", res)
	}
	if !strings.Contains(gotQuery, "by=model") {
		t.Fatalf("query not passed: %q", gotQuery)
	}
}
