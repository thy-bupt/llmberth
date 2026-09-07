package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/THY17308111153/llmberth/internal/keys"
	"github.com/THY17308111153/llmberth/internal/scaffold"
	"github.com/THY17308111153/llmberth/template"
	"github.com/spf13/cobra"
)

// AdminTokenFileName is where the CLI keeps its copy of the generated app's
// admin token (0600, gitignored by the generated project).
const AdminTokenFileName = ".llmberth-admin-token"

// safeDirName is the same allowlist scaffold.Options.Validate applies to the
// project name; gitInit re-checks defensively because the directory is used
// as a subprocess working directory.
var safeDirName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func newInitCmd() *cobra.Command {
	var (
		flagProvider string
		flagUI       string
		flagModule   string
		flagForce    bool
	)

	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Generate a new llmberth project you fully own",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, args, flagProvider, flagUI, flagModule, flagForce)
		},
	}
	cmd.Flags().StringVar(&flagProvider, "provider", "", "upstream provider: "+strings.Join(scaffold.Providers(), ", "))
	cmd.Flags().StringVar(&flagUI, "ui", "", "bundled UI: "+strings.Join(scaffold.UIs(), ", "))
	cmd.Flags().StringVar(&flagModule, "module", "", "go module path of the generated project (default: project name)")
	cmd.Flags().BoolVar(&flagForce, "force", false, "allow generating into a non-empty directory")
	return cmd
}

func runInit(cmd *cobra.Command, args []string, flagProvider, flagUI, flagModule string, force bool) error {
	out := cmd.OutOrStdout()
	interactive := isTTY(os.Stdin) && isTTY(os.Stdout)

	opts := scaffold.Options{Name: "", Provider: flagProvider, UI: flagUI, Module: flagModule}

	// Name: positional arg, else ask (TTY) else a sensible default.
	if len(args) == 1 {
		opts.Name = args[0]
	} else if interactive {
		name, err := askText("Project name", "my-llm-app", stdinReader, out)
		if err != nil {
			return err
		}
		opts.Name = name
	} else {
		opts.Name = "my-llm-app"
	}

	// The two questions (provider / ui), per plan §3.1. Flags skip them.
	if opts.Provider == "" || opts.UI == "" {
		if !interactive {
			missing := []string{}
			if opts.Provider == "" {
				missing = append(missing, "--provider")
			}
			if opts.UI == "" {
				missing = append(missing, "--ui")
			}
			return fmt.Errorf("non-interactive session: pass %s (choices: provider=%s; ui=%s)",
				strings.Join(missing, " "), strings.Join(scaffold.Providers(), "|"), strings.Join(scaffold.UIs(), "|"))
		}
		if opts.Provider == "" {
			ids := scaffold.Providers()
			display := make([]string, len(ids))
			for i, id := range ids {
				p, _ := scaffold.Preset(id)
				display[i] = p.DisplayName
			}
			choice, err := askChoice("Which LLM provider will the app talk to?", ids, display, stdinReader, out)
			if err != nil {
				return err
			}
			opts.Provider = choice
		}
		if opts.UI == "" {
			uis := scaffold.UIs()
			display := []string{
				"Single-page chat UI (served by the app)",
				"None (API only)",
			}
			choice, err := askChoice("Bundle a UI?", uis, display, stdinReader, out)
			if err != nil {
				return err
			}
			opts.UI = choice
		}
	}

	opts.FillDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}

	preset, _ := scaffold.Preset(opts.Provider)
	targetDir := opts.TargetDir(".")
	fmt.Fprintf(out, "Generating %q (provider=%s, ui=%s, db=postgres)\n", opts.Name, opts.Provider, opts.UI)

	adminToken, err := keys.NewAdminToken()
	if err != nil {
		return err
	}
	dbPassword, err := keys.NewAdminToken() // 256-bit hex; same primitive as the admin token
	if err != nil {
		return err
	}
	engine := &scaffold.Engine{
		FS:              template.FS,
		Root:            template.Root,
		TemplateVersion: version,
		CLIVersion:      version,
	}
	files, manifest, err := engine.Generate(opts, targetDir, force, adminToken, dbPassword)
	if err != nil {
		return err
	}

	// Persist the CLI's copy of the admin token (0600, gitignored by the
	// generated project). The generated app reads its own copy from .env.
	tokenPath := filepath.Join(targetDir, AdminTokenFileName)
	if err := os.WriteFile(tokenPath, []byte(adminToken+"\n"), 0o600); err != nil {
		return fmt.Errorf("write admin token file: %w", err)
	}

	// Version control: init the generated project's own git repo.
	if err := gitInit(targetDir); err != nil {
		fmt.Fprintf(out, "warning: git init skipped (%v) — run it manually later\n", err)
	}

	fmt.Fprintf(out, "Wrote %d files. Template version %s.\n\n", len(files), manifest.TemplateVersion)
	printNextSteps(out, opts, preset)
	return nil
}

func newInitGitCommand() *exec.Cmd {
	c := exec.Command("git", "init", "-q")
	return c
}

func newAddGitCommand() *exec.Cmd {
	c := exec.Command("git", "add", "-A")
	return c
}

func newCommitGitCommand() *exec.Cmd {
	c := exec.Command("git", "commit", "-q", "-m", "chore: llmberth init scaffold")
	return c
}

// gitInit creates the generated project's own git repository with an initial
// commit. Every command is built from a dedicated constructor with
// compile-time constant arguments; no user data ever enters argv. The
// project directory is re-validated against a strict allowlist before being
// used as the subprocess working directory.
func gitInit(dir string) error {
	base := filepath.Base(dir)
	if !safeDirName.MatchString(base) {
		return fmt.Errorf("project dir %q does not match the safe-name allowlist", base)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil // already a repo
	}

	identity := []string{
		"GIT_AUTHOR_NAME=llmberth",
		"GIT_COMMITTER_NAME=llmberth",
		"GIT_AUTHOR_EMAIL=llmberth@localhost",
		"GIT_COMMITTER_EMAIL=llmberth@localhost",
	}
	for _, newCmd := range []func() *exec.Cmd{newInitGitCommand, newAddGitCommand, newCommitGitCommand} {
		c := newCmd()
		c.Dir = dir
		c.Env = append(os.Environ(), identity...)
		if outB, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %s", c.Path, strings.TrimSpace(string(outB)))
		}
	}
	return nil
}

func printNextSteps(out io.Writer, opts scaffold.Options, preset scaffold.ProviderPreset) {
	fmt.Fprintf(out, "Next steps:\n")
	if preset.NeedsAPIKey {
		fmt.Fprintf(out, "  1. Put your %s in %s/.env (it was created with the field empty)\n", preset.APIKeyEnv, opts.Name)
	} else if opts.Provider == "fake" {
		fmt.Fprintf(out, "  1. No upstream key needed — the fake provider answers locally\n")
	} else {
		fmt.Fprintf(out, "  1. Point the app at your instance: set the upstream base_url/model in %s/.env if needed\n", opts.Name)
	}
	fmt.Fprintf(out, "  2. cd %s\n", opts.Name)
	fmt.Fprintf(out, "  3. docker compose up -d --wait     # starts postgres + app (dev profile)\n")
	fmt.Fprintf(out, "  4. curl -s http://127.0.0.1:%d/healthz\n", scaffold.DefaultAppPort)
	fmt.Fprintf(out, "  5. curl -s http://127.0.0.1:%d/v1/chat/completions \\\n", scaffold.DefaultAppPort)
	fmt.Fprint(out, `       -H "Authorization: Bearer lbt_live_YOUR_API_KEY" -H "Content-Type: application/json" \`+"\n")
	fmt.Fprintf(out, `       -d '{"model":"%s","messages":[{"role":"user","content":"hello"}]}'`+"\n", preset.DefaultModel)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Keys live in the app's database; manage them via the loopback admin API (token file: %s, 0600).\n", AdminTokenFileName)
	fmt.Fprintln(out, "No telemetry. The generated code has zero runtime dependency on llmberth — it is yours.")
}
