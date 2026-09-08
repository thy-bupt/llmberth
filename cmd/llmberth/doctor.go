package main

import (
	"fmt"
	"os"

	"github.com/thy-bupt/llmberth/internal/runtime"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var (
		path     string
		security bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Environment health check (docker, project structure, .env completeness, reachability)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return fmt.Errorf("%w (doctor needs a project; use --path to point at one)", err)
			}
			if security {
				// The 12-point security audit lands with M4 (v0.4); refuse
				// quietly rather than pretending to check.
				return fmt.Errorf("--security arrives with v0.4 (M4); basic checks only in v0.2")
			}
			ctx, cancel := withSignalCtx()
			defer cancel()
			res, err := runtime.RunDoctor(ctx, dir)
			if err != nil {
				return err
			}
			res.Render(cmd.OutOrStdout())
			if res.HasFail() {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	cmd.Flags().BoolVar(&security, "security", false, "run the 12-point security audit (v0.4)")
	return cmd
}
