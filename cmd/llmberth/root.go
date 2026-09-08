package main

import (
	"github.com/spf13/cobra"
)

// version is stamped at build time via
// -ldflags "-X main.version=$(git describe --tags)".
var version = "dev"

// NewRootCmd assembles the llmberth command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "llmberth",
		Short: "Self-hosted LLM app generate + operate toolkit",
		Long: `llmberth scaffolds a production-grade, OpenAI-compatible Go backend
that you fully own — source, data and all — and manages its whole life from
the same binary: dev, up, stop, logs, keys, usage, doctor.

Generated apps ship with token accounting, per-key budgets and rate limits,
instant key revocation, audit logging and log redaction out of the box.
No telemetry. No platform lock-in.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version
	root.SetVersionTemplate("llmberth {{.Version}}\n")
	root.CompletionOptions.HiddenDefaultCmd = true
	root.AddCommand(
		newInitCmd(),
		newDevCmd(),
		newUpCmd(),
		newStopCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newKeysCmd(),
		newUsageCmd(),
		newDoctorCmd(),
		newTuiCmd(),
	)
	return root
}

// Execute runs the root command; the caller decides how to surface errors.
func Execute() error {
	return NewRootCmd().Execute()
}
