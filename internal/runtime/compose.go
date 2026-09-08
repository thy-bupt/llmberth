// Package runtime drives the generated project's docker compose stack with
// subprocess calls (plan §2.1). Every exec.Command here is built from a
// dedicated constructor with compile-time constant arguments; varying inputs
// (project dir, service names) enter only via cmd.Dir, never argv.
package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Service is a compose service of the generated project. The set is fixed
// by the v0.1 template; unknown names are rejected (fail closed).
type Service string

// Compose services of the v0.1 template; the set is fixed and fail-closed.
const (
	ServiceApp      Service = "app"
	ServicePostgres Service = "postgres"
)

// ParseService validates a user-provided service name.
func ParseService(name string) (Service, error) {
	switch Service(name) {
	case ServiceApp, ServicePostgres:
		return Service(name), nil
	}
	return "", fmt.Errorf("unknown service %q (valid: app, postgres)", name)
}

func composeUpCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "up", "-d", "--wait")
	c.Dir = dir
	return c
}

func composeDownCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "down")
	c.Dir = dir
	return c
}

func composePsCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "ps", "--format", "json")
	c.Dir = dir
	return c
}

func composeLogsCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "logs", "--tail=200")
	c.Dir = dir
	return c
}

func composeLogsFollowCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "logs", "-f", "--tail=50")
	c.Dir = dir
	return c
}

func composeLogsServiceCmd(dir string, service Service) *exec.Cmd {
	c := exec.Command("docker", "compose", "logs", "--tail=200", string(service)) //nolint:gosec // service is allowlisted by ParseService, not raw user data
	c.Dir = dir
	return c
}

func composeLogsServiceFollowCmd(dir string, service Service) *exec.Cmd {
	c := exec.Command("docker", "compose", "logs", "-f", "--tail=50", string(service)) //nolint:gosec // allowlisted service name
	c.Dir = dir
	return c
}

func composeUpPostgresCmd(dir string) *exec.Cmd {
	c := exec.Command("docker", "compose", "up", "-d", "--wait", "postgres")
	c.Dir = dir
	return c
}

func run(c *exec.Cmd) ([]byte, error) {
	out, err := c.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w\n%s", strings.Join(c.Args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Up brings the whole stack up and waits for health. extraEnv (KEY=VALUE
// pairs, e.g. keychain overlays) is merged into the compose subprocess
// environment so ${VAR} interpolation can pick it up.
func Up(ctx context.Context, dir string, extraEnv []string) error {
	c := composeUpCmd(dir)
	if len(extraEnv) > 0 {
		c.Env = append(os.Environ(), extraEnv...)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := run(c)
	return err
}

// Stop tears the stack down (containers stopped and removed; volume kept).
func Stop(_ context.Context, dir string) error {
	_, err := run(composeDownCmd(dir))
	return err
}

// StatusRaw returns `docker compose ps --format json` output for parsing.
func StatusRaw(_ context.Context, dir string) ([]byte, error) {
	return run(composePsCmd(dir))
}

// StatusRow is one service line of a friendly status table.
type StatusRow struct {
	Service   string
	State     string
	Health    string
	Published string
}

// ParseStatus decodes `docker compose ps --format json` (JSON Lines) into
// friendly rows; host-published ports are summarized "127.0.0.1:host".
func ParseStatus(raw []byte) ([]StatusRow, error) {
	var rows []StatusRow
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj struct {
			Service    string `json:"Service"`
			State      string `json:"State"`
			Health     string `json:"Health"`
			Publishers []struct {
				URL          string `json:"URL"`
				PublishedPort int   `json:"PublishedPort"`
				TargetPort   int    `json:"TargetPort"`
			} `json:"Publishers"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, err
		}
		pubs := make([]string, 0, len(obj.Publishers))
		for _, p := range obj.Publishers {
			pub := p.URL
			if pub == "" {
				pub = "0.0.0.0"
			}
			pubs = append(pubs, fmt.Sprintf("%s:%d->%d", pub, p.PublishedPort, p.TargetPort))
		}
		rows = append(rows, StatusRow{
			Service:   obj.Service,
			State:     obj.State,
			Health:    obj.Health,
			Published: strings.Join(pubs, ", "),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no compose services found")
	}
	return rows, nil
}

// Logs prints recent logs; with follow it streams until ctx is done. Writer
// targets make it usable by both the CLI and the TUI logs page.
func Logs(ctx context.Context, dir string, service Service, follow bool, stdout, stderr io.Writer) error {
	var c *exec.Cmd
	switch {
	case service != "" && follow:
		c = composeLogsServiceFollowCmd(dir, service)
	case service != "":
		c = composeLogsServiceCmd(dir, service)
	case follow:
		c = composeLogsFollowCmd(dir)
	default:
		c = composeLogsCmd(dir)
	}
	c.Stdout = stdout
	c.Stderr = stderr
	if !follow {
		return c.Run()
	}
	// Follow: kill the subprocess when ctx is cancelled (Ctrl-C on the CLI).
	if err := c.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		close(done)
	}()
	err := c.Wait()
	<-done
	if err != nil && ctx.Err() != nil {
		return nil // interrupted by our own shutdown, not a failure
	}
	return err
}

// UpPostgres starts only the postgres service (used by dev).
func UpPostgres(_ context.Context, dir string) error {
	_, err := run(composeUpPostgresCmd(dir))
	return err
}

// ComposeAvailable reports whether docker + compose v2 are usable.
func ComposeAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH; install Docker (or OrbStack) first")
	}
	c := exec.Command("docker", "compose", "version")
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose v2 plugin not available: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// FollowLines streams lines from r to stdout until EOF (helper for dev).
func FollowLines(r *bufio.Reader, fn func(line string)) error {
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			fn(strings.TrimRight(line, "\n"))
		}
		if err != nil {
			return err
		}
	}
}
