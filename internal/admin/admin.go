// Package admin is the CLI-side client for the generated app's loopback
// admin API (plan §2.3): the CLI never touches the app's Postgres directly.
// The token lives in <project>/.llmberth-admin-token (0600, gitignored);
// a missing or rejected token fails closed.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to the loopback admin API.
type Client struct {
	BaseURL string // e.g. http://127.0.0.1:8090
	Token   string
	HTTP    *http.Client
}

// TokenFileName is the 0600 file holding the admin token, written by init.
const TokenFileName = ".llmberth-admin-token"

// LoadToken reads the admin token for a project directory (fail closed on
// missing file).
func LoadToken(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, TokenFileName))
	if err != nil {
		return "", fmt.Errorf("admin token file missing (%s); a loopback admin API token is required — regenerate the project or restore the file", TokenFileName)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("admin token file %s is empty", TokenFileName)
	}
	return token, nil
}

// NewClient builds a client from a project directory (manifest provides the
// admin port; token file provides the credential).
func NewClient(projectDir string, manifest Porter, httpClient *http.Client) (*Client, error) {
	token, err := LoadToken(projectDir)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", manifest.AdminPort()),
		Token:   token,
		HTTP:    httpClient,
	}, nil
}

// Porter is the slice of the manifest the client needs.
type Porter interface {
	AdminPort() int
}

// Key is the non-secret view of an API key.
type Key struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Prefix       string     `json:"prefix"`
	RateLimitRPS int        `json:"rate_limit_rps"`
	BudgetUSD    *float64   `json:"budget_usd"`
	RevokedAt    *time.Time `json:"revoked_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CreateKeyResult is returned by CreateKey: Full is shown exactly once.
type CreateKeyResult struct {
	ID      string  `json:"id"`
	Prefix  string  `json:"prefix"`
	Full    string  `json:"key"`
	Warning string  `json:"warning"`
	Budget  float64 `json:"-"`
}

// CreateKeyRequest is the payload for key creation.
type CreateKeyRequest struct {
	Name         string   `json:"name"`
	BudgetUSD    *float64 `json:"budget_usd"`
	RateLimitRPS int      `json:"rate_limit_rps"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("admin API unreachable (%s): %w — is the app running?", c.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("admin API %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("admin API %s: decode response: %w", path, err)
		}
	}
	return nil
}

// ListKeys returns the non-secret key metadata.
func (c *Client) ListKeys(ctx context.Context) ([]Key, error) {
	var keys []Key
	if err := c.do(ctx, http.MethodGet, "/admin/keys", nil, &keys); err != nil {
		return nil, err
	}
	if keys == nil {
		keys = []Key{}
	}
	return keys, nil
}

// CreateKey mints a key; the full value is returned exactly once.
func (c *Client) CreateKey(ctx context.Context, req CreateKeyRequest) (CreateKeyResult, error) {
	var res CreateKeyResult
	err := c.do(ctx, http.MethodPost, "/admin/keys", req, &res)
	return res, err
}

// RevokeKey immediately invalidates the key (the app clears its cache).
func (c *Client) RevokeKey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/admin/keys/"+id+"/revoke", nil, nil)
}

// UsagePoint is one aggregation row (costs are estimates).
type UsagePoint struct {
	Group            string  `json:"group"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// UsageResponse is the aggregate envelope.
type UsageResponse struct {
	By         string       `json:"by"`
	Since      string       `json:"since"`
	Points     []UsagePoint `json:"points"`
	Note       string       `json:"note"`
	MonthSpend float64      `json:"month_spend"`
}

// Usage queries the ledger aggregate. by is "", "key" or "model".
func (c *Client) Usage(ctx context.Context, by string, since time.Time) (UsageResponse, error) {
	var res UsageResponse
	path := "/admin/usage?by=" + by + "&since=" + since.UTC().Format(time.RFC3339)
	err := c.do(ctx, http.MethodGet, path, nil, &res)
	return res, err
}

// Health pings the admin API (also proves the app process is alive).
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/admin/health", nil, nil)
}
