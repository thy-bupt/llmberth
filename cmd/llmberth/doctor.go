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
				// The twelve-item security audit (plan §5.7).
				checks, err := runtime.CheckSecurity(dir)
				if err != nil {
					return err
				}
				res := runtime.DoctorResult{Checks: checks}
				res.Render(cmd.OutOrStdout())
				if res.HasFail() {
					os.Exit(1)
				}
				return nil
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
