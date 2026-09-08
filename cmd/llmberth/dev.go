package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/THY17308111153/llmberth/internal/runtime"
	"github.com/spf13/cobra"
)

func newDevCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run the app on the host with hot reload (air) against the dev postgres",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDev(cmd, path)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}

func runDev(cmd *cobra.Command, path string) error {
	dir, err := projectDir(path)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	// 1. .env completeness first (M2 acceptance: missing keys reported).
	ctx, cancel := withSignalCtx()
	defer cancel()
	res, err := runtime.RunDoctor(ctx, dir)
	if err != nil {
		return err
	}
	for _, c := range res.Checks {
		if c.Sev == runtime.Fail && strings.Contains(c.Name, ".env") {
			return fmt.Errorf("dev aborted: %s — %s", c.Name, c.Detail)
		}
	}

	// 2. postgres up (loopback-published port only).
	fmt.Fprintln(out, "Ensuring postgres (docker compose up -d --wait postgres)…")
	if err := runtime.ComposeAvailable(); err != nil {
		return err
	}
	if err := runtime.UpPostgres(ctx, dir); err != nil {
		return err
	}

	// 3. Host-side env: DATABASE_URL must point at the loopback-published
	// port, not the in-container hostname. Keychain upstream keys are
	// overlaid last so they win over .env (plan §5.1).
	envFile, err := parseEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		return err
	}
	appEnv := buildDevEnv(envFile)
	if overlay, err := keychainOverlay(); err == nil {
		appEnv = append(appEnv, overlay...)
	}

	// 4. air when available, otherwise `go run .` (still hot-restartable by
	// the operator; the message says which one).
	if _, err := exec.LookPath("air"); err == nil {
		c := exec.Command("air", "-c", ".air.toml")
		c.Dir = dir
		c.Env = append(os.Environ(), appEnv...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		fmt.Fprintln(out, "Running app with air (hot reload). Ctrl-C stops.")
		return c.Run()
	}

	c := exec.Command("go", "run", ".")
	c.Dir = dir
	c.Env = append(os.Environ(), appEnv...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	fmt.Fprintln(out, "air not found — falling back to `go run .` (no hot reload).")
	fmt.Fprintln(out, "Install air for hot reload:  go install github.com/air-verse/air@latest")
	return c.Run()
}

// parseEnvFile reads KEY=VALUE pairs from a .env file.
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .env: %w", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out, nil
}

// buildDevEnv assembles the host-side app environment: loopback DATABASE_URL,
// loopback listeners, secrets and provider knobs carried over from .env.
// Values are never printed.
func buildDevEnv(envFile map[string]string) []string {
	pairs := []string{
		"DATABASE_URL=" + firstNonEmpty(envFile["DATABASE_URL_HOST"], envFile["DATABASE_URL"]),
		"PORT=127.0.0.1:8080",
		"ADMIN_PORT=127.0.0.1:8090",
		"APP_ENV=dev",
	}
	for _, k := range []string{
		"ADMIN_TOKEN", "LLM_PROVIDER", "LLM_BASE_URL", "LLM_MODEL",
		"LLM_ALLOWED_HOSTS", "RATE_LIMIT_RPS", "BUDGET_MONTHLY_USD",
		"LOG_MESSAGES", "CORS_ORIGINS", "OPENAI_API_KEY", "DEEPSEEK_API_KEY",
		"DASHSCOPE_API_KEY", "UPSTREAM_API_KEY",
	} {
		if v := envFile[k]; v != "" {
			pairs = append(pairs, k+"="+v)
		}
	}
	return pairs
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
