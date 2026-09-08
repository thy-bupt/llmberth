package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/THY17308111153/llmberth/internal/scaffold"
	tea "github.com/charmbracelet/bubbletea"
)

// stubProject writes a manifest + token file and returns the dir. The admin
// port is pointed at the stub server so the TUI's client (same internal
// construction as the CLI) talks to it.
func stubProject(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	hostPort := srv.URL[len("http://"):]

	dir := t.TempDir()
	m := scaffold.NewManifest(scaffold.Options{Name: "t", Module: "t", Provider: "fake", UI: "none", DB: "postgres"}, "dev", "dev")
	m.Services.App.AdminPort = atoiPort(hostPort)
	if err := scaffold.WriteManifest(filepath.Join(dir, scaffold.ManifestFileName), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".llmberth-admin-token"), []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func atoiPort(hostPort string) int {
	_, port, ok := strings.Cut(hostPort, ":")
	if !ok {
		return 0
	}
	var n int
	for _, c := range port {
		if c < '0' || c > '9' {
			continue
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func stubMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": "k1", "name": "demo", "prefix": "lbt_live_pref", "rate_limit_rps": 5,
				"budget_usd": nil, "revoked_at": nil, "created_at": time.Now().Format(time.RFC3339),
			}})
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "k2", "prefix": "lbt_live_newp", "key": "lbt_live_FULLKEYONCE"})
		}
	})
	mux.HandleFunc("/admin/keys/k1/revoke", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"k1","revoked":true}`))
	})
	mux.HandleFunc("/admin/usage", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"by": "", "since": "2026-09-01T00:00:00Z",
			"points":      []map[string]any{{"group": "", "requests": 7, "prompt_tokens": 14, "completion_tokens": 49, "cost_usd": 0.00154}},
			"month_spend": 0.00154, "note": "estimates",
		})
	})
	mux.HandleFunc("/admin/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func newTestModel(t *testing.T) *Model {
	t.Helper()
	dir := stubProject(t, stubMux())
	model, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return model.(*Model)
}

func keyMsg(s string) tea.KeyMsg {
	r := []rune(s)
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: r}
}

// TestDashboardRenders asserts the dashboard view pulls stack + usage +
// budget through internal packages (stub admin API).
func TestDashboardRenders(t *testing.T) {
	m := newTestModel(t)
	m.refreshSilent()
	v := m.View()
	for _, want := range []string{"Stack health", "Usage", "Budget", "llmberth"} {
		if !strings.Contains(v, want) {
			t.Errorf("dashboard view missing %q:\n%s", want, v)
		}
	}
	// Stack data comes from runtime (docker) — absent in tests, so the view
	// must degrade to a warning, not crash.
	if !strings.Contains(v, "not running") {
		t.Errorf("empty stack state not surfaced:\n%s", v)
	}
}

// TestCreateKeyFlow drives the keys page through the whole create flow: the
// full key is shown exactly once, then dismissed.
func TestCreateKeyFlow(t *testing.T) {
	m := newTestModel(t)
	m.refreshKeys()

	// Switch to keys tab, start creation.
	m.Update(keyMsg("2"))
	if m.tab != TabKeys {
		t.Fatalf("tab not keys")
	}
	// List must show only the prefix, never a full key.
	if v := m.keysView(); !strings.Contains(v, "lbt_live_pref") {
		t.Fatalf("key list missing prefix:\n%s", v)
	}

	m.Update(keyMsg("n"))
	if m.keyAction != "creating" {
		t.Fatalf("not in creating mode")
	}
	for _, ch := range "tui-key" {
		m.Update(keyMsg(string(ch)))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.keyAction != "show-created" || m.created == nil {
		t.Fatalf("create did not reach show-created")
	}
	if v := m.keysView(); !strings.Contains(v, "lbt_live_FULLKEYONCE") {
		t.Fatalf("full key not shown exactly-once:\n%s", v)
	}
	// Dismiss — full key must disappear from the view.
	m.Update(keyMsg("q"))
	if v := m.keysView(); strings.Contains(v, "lbt_live_FULLKEYONCE") {
		t.Fatalf("full key persisted after dismissal")
	}
}

// TestRevokeFlow selects the first key and confirms revoke.
func TestRevokeFlow(t *testing.T) {
	m := newTestModel(t)
	m.refreshKeys()
	m.Update(keyMsg("2"))
	m.Update(keyMsg("r"))
	if m.keyAction != "confirm-revoke" || m.revokeSid != "k1" {
		t.Fatalf("revoke confirm not armed: %+v", m)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.keyAction != "" {
		t.Fatalf("confirm did not finish")
	}
}

// TestLogsFilter checks the level filter logic on structured lines.
func TestLogsFilter(t *testing.T) {
	lines := []string{
		`{"level":"ERROR","msg":"boom"}`,
		`{"level":"INFO","msg":"ok"}`,
		`{"level":"WARN","msg":"careful"}`,
		`{"level":"DEBUG","msg":"spam"}`,
		"plain text line",
	}
	matches := map[string][]int{
		"ERROR": {0},
		"INFO":  {1},
		"WARN":  {2},
		"DEBUG": {3},
		"ALL":   {0, 1, 2, 3, 4},
	}
	for filter, wantIdx := range matches {
		var got []int
		for i, l := range lines {
			if levelMatches(l, filter) {
				got = append(got, i)
			}
		}
		if len(got) != len(wantIdx) {
			t.Errorf("filter %s got %v want %v", filter, got, wantIdx)
		}
	}
}
