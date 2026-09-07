// Command llmberth is the single binary of the llmberth toolkit: it
// scaffolds a production-grade, OpenAI-compatible Go LLM backend that you
// fully own, and manages that project's whole life (dev/up/stop/logs/keys/
// usage/doctor).
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "llmberth:", err)
		os.Exit(1)
	}
}
