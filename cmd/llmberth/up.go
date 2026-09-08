package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/THY17308111153/llmberth/internal/admin"
	"github.com/THY17308111153/llmberth/internal/runtime"
	"github.com/THY17308111153/llmberth/internal/scaffold"
	"github.com/THY17308111153/llmberth/internal/usage"
	"github.com/spf13/cobra"
)

// projectDir resolves the llmberth project root for commands that require
// one: explicit --path, else walk up from cwd to .llmberth.yaml.
func projectDir(pathFlag string) (string, error) {
	if pathFlag != "" {
		return pathFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return scaffold.FindManifest(cwd)
}

func withSignalCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	return ctx, cancel
}

func newUpCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the project's compose stack (postgres + app) and wait for health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return err
			}
			// Keychain overlay: stored upstream keys win over .env without
			// touching it (plan §5.1). A broken keychain does not block up.
			var extra []string
			if overlay, err := keychainOverlay(); err == nil {
				extra = overlay
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "note: keychain unavailable (%v) — continuing with .env values\\n", err)
			}
			ctx, cancel := withSignalCtx()
			defer cancel()
			fmt.Fprintln(cmd.OutOrStdout(), "Starting stack (docker compose up -d --wait)…")
			if err := runtime.Up(ctx, dir, extra); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Stack is up. Public API on 127.0.0.1, admin API on 127.0.0.1 only.")
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}

func newStopCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop and remove the project's compose stack (volume kept)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Stopping stack (docker compose down)…")
			return runtime.Stop(context.Background(), dir)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show compose service health and ports",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return err
			}
			raw, err := runtime.StatusRaw(context.Background(), dir)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			services, err := runtime.ParseStatus(raw)
			if err != nil {
				// Fall back to raw output rather than failing on an
				// unexpected docker format.
				_, _ = out.Write(raw)
				fmt.Fprintln(out)
				return nil
			}
			fmt.Fprintf(out, "%-12s %-10s %-28s %s\n", "SERVICE", "STATE", "HEALTH", "PUBLISHED (host)")
			for _, s := range services {
				state := s.State
				if s.Health != "" && s.Health != "none" {
					state = state + " (" + s.Health + ")"
				}
				fmt.Fprintf(out, "%-12s %-10s %-28s %s\n", s.Service, state, "", s.Published)
			}
			// Budget line (plan §3: status surfaces the consumption ratio).
			// Show it only when the admin API is reachable; never fail the
			// status command over it.
			if mm, err := scaffold.LoadManifest(filepath.Join(dir, scaffold.ManifestFileName)); err == nil {
				if client, err := admin.NewClient(dir, mm, nil); err == nil {
					monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
					if res, err := client.Usage(context.Background(), "", monthStart); err == nil {
						usage.RenderBudget(out, usage.Summarize(res, mm.Budget.MonthlyUSD))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}

func newLogsCmd() *cobra.Command {
	var (
		path    string
		follow  bool
		service string
	)
	cmd := &cobra.Command{
		Use:   "logs [-f] [service]",
		Short: "Show stack logs (app or postgres), optionally streaming",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return err
			}
			svc := runtime.Service("")
			if len(args) == 1 {
				svc, err = runtime.ParseService(args[0])
				if err != nil {
					return err
				}
			}
			ctx, cancel := withSignalCtx()
			defer cancel()
			return runtime.Logs(ctx, dir, svc, follow, os.Stdout, os.Stderr)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream logs until interrupted")
	cmd.Flags().StringVar(&service, "service", "", "shorthand for the positional service argument")
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}
