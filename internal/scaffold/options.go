// Package scaffold renders the llmberth project template: option
// validation, the .llmberth.yaml manifest and the embed-based template
// engine that writes a complete, self-owned Go application to disk.
package scaffold

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Options fully describes one scaffold invocation. It is the single source of
// truth passed to the template engine.
type Options struct {
	// Name is the project name; the target directory is created with it.
	Name string `json:"name"`
	// Module is the go module path of the generated project.
	Module string `json:"module"`
	// Provider is one of the preset IDs (openai, deepseek, qwen, vllm, ollama, fake).
	Provider string `json:"provider"`
	// UI is "single-page" or "none".
	UI string `json:"ui"`
	// DB is fixed to "postgres" in v0.1.
	DB string `json:"db"`
}

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	moduleRe = regexp.MustCompile(`^[a-zA-Z0-9._~/-]+$`)
)

// ProviderPreset carries everything the template needs to know about the
// selected upstream provider.
type ProviderPreset struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	BaseURL      string `json:"base_url"`
	APIKeyEnv    string `json:"api_key_env"` // env var the .env.example hints at; empty when unused
	NeedsAPIKey  bool   `json:"needs_api_key"`
	DefaultModel string `json:"default_model"`
}

var providerPresets = map[string]ProviderPreset{
	"openai": {
		ID:           "openai",
		DisplayName:  "OpenAI",
		BaseURL:      "https://api.openai.com/v1",
		APIKeyEnv:    "OPENAI_API_KEY",
		NeedsAPIKey:  true,
		DefaultModel: "gpt-4o-mini",
	},
	"deepseek": {
		ID:           "deepseek",
		DisplayName:  "DeepSeek",
		BaseURL:      "https://api.deepseek.com/v1",
		APIKeyEnv:    "DEEPSEEK_API_KEY",
		NeedsAPIKey:  true,
		DefaultModel: "deepseek-chat",
	},
	"qwen": {
		ID:           "qwen",
		DisplayName:  "Qwen (DashScope compatible mode)",
		BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKeyEnv:    "DASHSCOPE_API_KEY",
		NeedsAPIKey:  true,
		DefaultModel: "qwen-plus",
	},
	"vllm": {
		ID:           "vllm",
		DisplayName:  "vLLM (self-hosted)",
		BaseURL:      "http://127.0.0.1:8000/v1",
		APIKeyEnv:    "",
		NeedsAPIKey:  false,
		DefaultModel: "custom-model",
	},
	"ollama": {
		ID:           "ollama",
		DisplayName:  "Ollama (self-hosted)",
		BaseURL:      "http://127.0.0.1:11434/v1",
		APIKeyEnv:    "",
		NeedsAPIKey:  false,
		DefaultModel: "llama3.1",
	},
	"fake": {
		ID:           "fake",
		DisplayName:  "Fake (built-in, no network, for demos and tests)",
		BaseURL:      "",
		APIKeyEnv:    "",
		NeedsAPIKey:  false,
		DefaultModel: "fake-chat",
	},
}

// Providers returns the preset IDs in stable display order.
func Providers() []string {
	return []string{"openai", "deepseek", "qwen", "vllm", "ollama", "fake"}
}

// Preset returns the ProviderPreset for an ID.
func Preset(id string) (ProviderPreset, error) {
	p, ok := providerPresets[id]
	if !ok {
		return ProviderPreset{}, fmt.Errorf("unknown provider %q (valid: %s)", id, strings.Join(Providers(), ", "))
	}
	return p, nil
}

// UIs returns the supported ui option values.
func UIs() []string {
	return []string{"single-page", "none"}
}

// FillDefaults completes optional fields (module derives from name).
func (o *Options) FillDefaults() {
	if o.Module == "" {
		o.Module = o.Name
	}
	if o.DB == "" {
		o.DB = "postgres"
	}
}

// Validate checks the whole option set.
func (o *Options) Validate() error {
	if o.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if !nameRe.MatchString(o.Name) {
		return fmt.Errorf("project name %q must match %v (lowercase, digits, '-' or '_')", o.Name, nameRe)
	}
	if o.Module == "" {
		return fmt.Errorf("module path is required")
	}
	if !moduleRe.MatchString(o.Module) || strings.HasPrefix(o.Module, "/") || strings.HasPrefix(o.Module, ".") {
		return fmt.Errorf("module path %q must be a valid go module path", o.Module)
	}
	if _, err := Preset(o.Provider); err != nil {
		return err
	}
	validUI := false
	for _, ui := range UIs() {
		if o.UI == ui {
			validUI = true
			break
		}
	}
	if !validUI {
		return fmt.Errorf("ui option must be one of: %s", strings.Join(UIs(), ", "))
	}
	if o.DB != "postgres" {
		return fmt.Errorf("db option: only postgres is supported in v0.1")
	}
	return nil
}

// TargetDir returns the absolute-ish directory the project is generated into,
// relative to the working directory.
func (o *Options) TargetDir(base string) string {
	return filepath.Join(base, o.Name)
}
