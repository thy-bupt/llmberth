package main

import (
	"fmt"
	"os"

	"github.com/thy-bupt/llmberth/internal/keys"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// newUpstreamCmd manages upstream provider API keys in the keychain (plan
// §5.1): OS keychain when available, AES-GCM file fallback. Stored keys are
// overlaid into dev/up environments; values never persist in .env.
func newUpstreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upstream",
		Short: "Manage upstream provider API keys in the keychain (not .env)",
	}
	cmd.AddCommand(newUpstreamSetCmd(), newUpstreamGetCmd(), newUpstreamDeleteCmd(), newUpstreamListCmd())
	return cmd
}

func openKeychain() (keys.Keychain, error) {
	return keys.OpenKeychain("")
}

func newUpstreamSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [env-name]",
		Short: "Store an upstream API key (prompted, no echo; never logged)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !keys.IsUpstreamEnv(name) {
				return fmt.Errorf("unknown upstream env name %q (valid: %s)", name, keys.UpstreamEnvNames())
			}
			kc, err := openKeychain()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Paste the %s value (input is hidden): ", name)
			value, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("read password: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if len(value) == 0 {
				return fmt.Errorf("empty value; nothing stored")
			}
			if err := kc.Set(name, string(value)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s stored in the keychain (name only, value never shown again).\n", name)
			return nil
		},
	}
	return cmd
}

func newUpstreamGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [env-name]",
		Short: "Print a stored upstream key (you asked for it — but logging it is on you)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := openKeychain()
			if err != nil {
				return err
			}
			v, err := kc.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), v)
			return nil
		},
	}
	return cmd
}

func newUpstreamDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [env-name]",
		Short: "Remove a stored upstream key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := openKeychain()
			if err != nil {
				return err
			}
			if err := kc.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s removed from the keychain.\n", args[0])
			return nil
		},
	}
	return cmd
}

func newUpstreamListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored upstream key names (values never shown)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			kc, err := openKeychain()
			if err != nil {
				return err
			}
			names, err := kc.List()
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No upstream keys stored.")
				return nil
			}
			for _, n := range names {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		},
	}
	return cmd
}

// keychainOverlay returns KEY=VALUE pairs for stored upstream keys; dev/up
// merge these over the project .env so keychain values win without touching
// .env (plan §5.1: no plaintext persistence).
func keychainOverlay() ([]string, error) {
	kc, err := openKeychain()
	if err != nil {
		return nil, err
	}
	return keys.Overlay(kc)
}
