package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/thy-bupt/llmberth/internal/admin"
	"github.com/thy-bupt/llmberth/internal/scaffold"
	"github.com/thy-bupt/llmberth/internal/usage"
	"github.com/spf13/cobra"
)

func newUsageCmd() *cobra.Command {
	var (
		path  string
		by    string
		since string
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Ledger aggregates: requests, tokens, estimated cost (by key or model)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, manifest, err := adminClientAndManifest(path)
			if err != nil {
				return err
			}
			window, err := parseSince(since)
			if err != nil {
				return err
			}
			res, err := client.Usage(cmd.Context(), by, window)
			if err != nil {
				return err
			}
			s := usage.Summarize(res, manifest.Budget.MonthlyUSD)
			s.Render(cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	cmd.Flags().StringVar(&by, "by", "", "group rows: key or model (default: totals)")
	cmd.Flags().StringVar(&since, "since", "", "window start: YYYY-MM-DD or RFC3339 (default: first day of current month)")
	return cmd
}

// adminClientAndManifest builds the admin client and returns the manifest
// (for budget figures) together.
func adminClientAndManifest(path string) (*admin.Client, *scaffold.Manifest, error) {
	dir, err := projectDir(path)
	if err != nil {
		return nil, nil, err
	}
	m, err := scaffold.LoadManifest(filepath.Join(dir, scaffold.ManifestFileName))
	if err != nil {
		return nil, nil, err
	}
	client, err := admin.NewClient(dir, m, nil)
	if err != nil {
		return nil, nil, err
	}
	return client, &m, nil
}

// parseSince accepts YYYY-MM-DD or RFC3339; defaults to the 1st of the
// current month (aligned with the monthly budget window).
func parseSince(raw string) (time.Time, error) {
	if raw == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since must be YYYY-MM-DD or RFC3339")
	}
	return t, nil
}
