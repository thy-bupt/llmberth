package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thy-bupt/llmberth/internal/scaffold"
)

// writeProjectFiles drops the given files into a temp project dir.
func writeProjectFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// baseCompose is the shape every generated project's dev compose has.
const baseCompose = `
services:
  postgres:
    image: postgres:16-alpine@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685
    ports:
      - "127.0.0.1:${POSTGRES_PORT:-15432}:5432"
  app:
    image: app
    ports:
      - "127.0.0.1:${APP_PORT:-8080}:8080"
      - "127.0.0.1:${ADMIN_PORT:-8090}:8090"
    read_only: true
    tmpfs:
      - /tmp
`

const baseProd = `
services:
  app:
    ports: !reset []
  postgres:
    ports: !reset []
  caddy:
    image: caddy:2-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648
    ports:
      - "80:80"
      - "443:443"
`

const baseDockerfile = `
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
RUN echo build
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
USER nonroot:nonroot
`

const baseEnv = "POSTGRES_PASSWORD=x\nDATABASE_URL=postgres://x\nADMIN_TOKEN=tok123\nLLM_PROVIDER=fake\nLLM_MODEL=fake-chat\nLOG_MESSAGES=0\n"

const baseMigration = `
CREATE TABLE IF NOT EXISTS api_keys (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash       text NOT NULL UNIQUE,
    prefix         text NOT NULL,
    scopes         text[] NOT NULL DEFAULT '{chat}',
    rate_limit_rps integer NOT NULL DEFAULT 5,
    budget_usd     numeric(12, 6),
    revoked_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);
`

const baseCI = "name: CI\njobs:\n  vuln:\n    steps:\n      - run: govulncheck ./...\n"

const baseCaddyfile = "{$APP_DOMAIN} {\n\treverse_proxy app:8080\n}\n"

func manifestFor(dir string, t *testing.T) {
	t.Helper()
	m := scaffold.NewManifest(scaffold.Options{Name: "t", Module: "t", Provider: "fake", UI: "none", DB: "postgres"}, "dev", "dev")
	if err := scaffold.WriteManifest(filepath.Join(dir, scaffold.ManifestFileName), m); err != nil {
		t.Fatal(err)
	}
}

func secureProject(t *testing.T) string {
	t.Helper()
	dir := writeProjectFiles(t, map[string]string{
		"docker-compose.yml":                baseCompose,
		"docker-compose.prod.yml":           baseProd,
		"Dockerfile":                        baseDockerfile,
		"Makefile":                          "build:\n\tgo build .\n",
		".env":                              baseEnv,
		".gitignore":                        ".env\n.llmberth-admin-token\n",
		".github/workflows/ci.yml":          baseCI,
		"internal/store/migrations/0001_init.up.sql": baseMigration,
		"Caddyfile":                         baseCaddyfile,
	})
	manifestFor(dir, t)
	return dir
}

func runAll(t *testing.T, dir string) map[string]Check {
	t.Helper()
	checks, err := CheckSecurity(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Check{}
	for _, c := range checks {
		out[c.Name] = c
	}
	return out
}

func gitInit(dir string) {
	c := exec.Command("git", "init", "-q") //nolint:gosec // constant argv; test fixture
	c.Dir = dir
	_ = c.Run()
}

func gitAddAll(dir string) {
	c := exec.Command("git", "add", "-A") //nolint:gosec // constant argv; test fixture
	c.Dir = dir
	_ = c.Run()
}

func gitCommit(dir string) {
	c := exec.Command("git", "-c", "user.name=llmberth", "-c", "user.email=llmberth@localhost", "commit", "-q", "-m", "probe") //nolint:gosec // constant argv; test fixture
	c.Dir = dir
	_ = c.Run()
}

// TestSecurityAllGreen: the generated-project shape passes every item.
func TestSecurityAllGreen(t *testing.T) {
	dir := secureProject(t)
	gitInit(dir)
	gitAddAll(dir)
	checks := runAll(t, dir)
	for _, name := range SecurityCheckNames {
		c, ok := checks[name]
		if !ok {
			t.Errorf("missing check %q", name)
			continue
		}
		if c.Sev != Ok {
			t.Errorf("%s: expected Ok, got %s (%s)", name, c.Sev, c.Detail)
		}
	}
}

// TestEnvTracked is the flip side of item 1.
func TestEnvTracked(t *testing.T) {
	dir := secureProject(t)
	gitInit(dir)
	// A probe commit first, then track .env explicitly.
	if err := os.WriteFile(filepath.Join(dir, ".probe"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddAll(dir)
	gitCommit(dir)
	gitAddAll(dir) // .env is gitignored by the template, so add -f is needed
	c := exec.Command("git", "add", "-f", ".env") //nolint:gosec // constant argv; test fixture
	c.Dir = dir
	_ = c.Run()
	got := runAll(t, dir)[SecurityCheckNames[0]]
	if got.Sev != Fail {
		t.Fatalf("tracked .env must fail item 1: %+v", got)
	}
}

// TestCredentialLiteral: a credential-shaped literal fails item 2.
func TestCredentialLiteral(t *testing.T) {
	dir := writeProjectFiles(t, map[string]string{
		"internal/hook.go": "package hook\nconst k = \"" + sampleOpenAIKey + "\"\n",
	})
	manifestFor(dir, t)
	got := runAll(t, dir)[SecurityCheckNames[1]]
	if got.Sev != Fail {
		t.Fatalf("credential literal must fail item 2: %+v", got)
	}
}

// Sample literals are assembled at runtime so the scanner itself never
// contains a full credential-shaped literal (the file it scans does).
const (
	// sk- + 32 hex-like chars
	sampleOpenAIKey = "sk-" + "abcdef0123456789abcdef0123456789"
	// ghp_ + 30 chars
	sampleGHPat = "ghp_" + "abcdefghijklmnopqrstuvwxyzABCD"
	// AIza + 33 chars
	sampleGoogleKey = "AIza" + "SyA1234567890abcdefghijklmnopqrstuv"
	// AKIA + 16 uppercase
	sampleAWSKey = "AKIA" + "IOSFODNN7EXAMPLE"
)

// TestScanFileForCredentials unit-checks the scanner itself.
func TestScanFileForCredentials(t *testing.T) {
	cases := []struct {
		input string
		hit   bool
	}{
		{sampleOpenAIKey, true},
		{sampleGHPat, true},
		{sampleGoogleKey, true},
		{sampleAWSKey, true},
		{`Bearer lbt_live_YOUR_API_KEY`, false}, // placeholder
		{`OPENAI_API_KEY=`, false},
		{`hello world`, false},
	}
	for _, tc := range cases {
		if got := ScanFileForCredentials([]byte(tc.input)); (got != "") != tc.hit {
			t.Errorf("input %q: hit=%q want %v", tc.input, got, tc.hit)
		}
	}
}

// TestIndivCheckFlips exercises each item's fail path by mutating one file.
func TestIndivCheckFlips(t *testing.T) {
	type flip struct {
		name     string
		mutate   func(dir string)
		itemName string
	}
	flips := []flip{
		{"admin public port", func(dir string) {
			p := filepath.Join(dir, "docker-compose.yml")
			s := strings.ReplaceAll(readFile(dir, "docker-compose.yml"), "127.0.0.1:${ADMIN_PORT:-8090}:8090", "0.0.0.0:${ADMIN_PORT:-8090}:8090")
			_ = os.WriteFile(p, []byte(s), 0o644)
		}, SecurityCheckNames[2]},
		{"postgres public", func(dir string) {
			p := filepath.Join(dir, "docker-compose.yml")
			s := strings.ReplaceAll(readFile(dir, "docker-compose.yml"), "127.0.0.1:${POSTGRES_PORT:-15432}:5432", "0.0.0.0:${POSTGRES_PORT:-15432}:5432")
			_ = os.WriteFile(p, []byte(s), 0o644)
		}, SecurityCheckNames[3]},
		{"unpinned image", func(dir string) {
			p := filepath.Join(dir, "Dockerfile")
			s := strings.Replace(readFile(dir, "Dockerfile"), "@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59", "", 1)
			_ = os.WriteFile(p, []byte(s), 0o644)
		}, SecurityCheckNames[4]},
		{"root user", func(dir string) {
			p := filepath.Join(dir, "Dockerfile")
			_ = os.WriteFile(p, []byte(strings.ReplaceAll(readFile(dir, "Dockerfile"), "USER nonroot:nonroot", "USER root")), 0o644)
		}, SecurityCheckNames[5]},
		{"no read_only", func(dir string) {
			p := filepath.Join(dir, "docker-compose.yml")
			_ = os.WriteFile(p, []byte(strings.ReplaceAll(readFile(dir, "docker-compose.yml"), "read_only: true", "")), 0o644)
		}, SecurityCheckNames[6]},
		{"no caddy", func(dir string) {
			p := filepath.Join(dir, "docker-compose.prod.yml")
			_ = os.WriteFile(p, []byte(strings.ReplaceAll(readFile(dir, "docker-compose.prod.yml"), "caddy:", "caddyx:")), 0o644)
		}, SecurityCheckNames[7]},
		{"log messages on", func(dir string) {
			p := filepath.Join(dir, ".env")
			_ = os.WriteFile(p, []byte(strings.ReplaceAll(readFile(dir, ".env"), "LOG_MESSAGES=0", "LOG_MESSAGES=1")), 0o600)
		}, SecurityCheckNames[8]},
		{"no budget", func(dir string) {
			m, err := scaffold.LoadManifest(filepath.Join(dir, scaffold.ManifestFileName))
			if err != nil {
				t.Fatal(err)
			}
			m.Budget.MonthlyUSD = 0
			if err := scaffold.WriteManifest(filepath.Join(dir, scaffold.ManifestFileName), m); err != nil {
				t.Fatal(err)
			}
		}, SecurityCheckNames[9]},
		{"full key column", func(dir string) {
			p := filepath.Join(dir, "internal/store/migrations/0001_init.up.sql")
			s := strings.Replace(readFile(dir, "internal/store/migrations/0001_init.up.sql"), "key_hash       text NOT NULL UNIQUE,", "full_key       text NOT NULL UNIQUE,", 1)
			_ = os.WriteFile(p, []byte(s), 0o644)
		}, SecurityCheckNames[10]},
		{"no vuln gate", func(dir string) {
			p := filepath.Join(dir, ".github/workflows/ci.yml")
			_ = os.WriteFile(p, []byte("name: CI\n"), 0o644)
		}, SecurityCheckNames[11]},
	}
	for _, f := range flips {
		dir := secureProject(t)
		f.mutate(dir)
		got, ok := runAll(t, dir)[f.itemName]
		if !ok {
			t.Errorf("%s: missing check", f.name)
			continue
		}
		if got.Sev != Fail {
			t.Errorf("%s: expected Fail, got %s (%s)", f.name, got.Sev, got.Detail)
		}
	}
}
