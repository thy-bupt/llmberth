package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// isTTY reports whether f is attached to an interactive terminal.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// stdinReader is a package-level indirection so tests can drive the prompts.
var stdinReader io.Reader = os.Stdin

// askChoice prints a numbered menu and returns the chosen option value.
// Empty input selects the default (options[0]).
func askChoice(prompt string, options []string, display []string, in io.Reader, out io.Writer) (string, error) {
	if len(display) != len(options) {
		return "", fmt.Errorf("internal: choice display mismatch")
	}
	fmt.Fprintf(out, "%s\n", prompt)
	for i, d := range display {
		fmt.Fprintf(out, "  %d) %s\n", i+1, d)
	}
	fmt.Fprintf(out, "Choose [1-%d, Enter=1]: ", len(options))

	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read choice: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return options[0], nil
		}
		n, convErr := strconv.Atoi(line)
		if convErr != nil || n < 1 || n > len(options) {
			fmt.Fprintf(out, "Please enter a number between 1 and %d: ", len(options))
			continue
		}
		return options[n-1], nil
	}
}

// askText prompts for a free-text answer; empty input returns def.
func askText(prompt, def string, in io.Reader, out io.Writer) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", prompt, def)
	} else {
		fmt.Fprintf(out, "%s: ", prompt)
	}
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}
