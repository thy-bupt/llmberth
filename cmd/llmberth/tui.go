package main

import (
	"fmt"

	"github.com/thy-bupt/llmberth/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// newTuiCmd enters the operator console. Rendering happens inside the tui
// package; this command only resolves the project and hands over — same
// internal-package data paths as every other command (plan §3.2 iron rule).
func newTuiCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Operator console: dashboard / keys / logs (TUI)",
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := projectDir(path)
			if err != nil {
				return err
			}
			model, err := tui.New(dir)
			if err != nil {
				return fmt.Errorf("tui init: %w", err)
			}
			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "project directory (default: nearest .llmberth.yaml)")
	return cmd
}
