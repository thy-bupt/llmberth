package scaffold

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ManifestVersion is the current schema version of .llmberth.yaml.
const ManifestVersion = 1

// Manifest is the project manifest every generated project carries at its
// root (.llmberth.yaml). CLI project discovery, doctor and (later) upgrade
// all depend on it. It must never contain secrets.
type Manifest struct {
	Version         int     `yaml:"version"`
	GeneratedBy     string  `yaml:"generated_by"`     // CLI version at generation time
	TemplateVersion string  `yaml:"template_version"` // template version, upgrade diff basis
	Options         Options `yaml:"options"`
	Services        struct {
		App struct {
			Port      int `yaml:"port"`
			AdminPort int `yaml:"admin_port"`
		} `yaml:"app"`
		Postgres struct {
			Version string `yaml:"version"`
		} `yaml:"postgres"`
	} `yaml:"services"`
	Budget struct {
		MonthlyUSD       float64 `yaml:"monthly_usd"`
		PerKeyDefaultUSD float64 `yaml:"per_key_default_usd"`
	} `yaml:"budget"`
}

// DefaultPorts are the v0.1 fixed service ports.
const (
	DefaultAppPort      = 8080
	DefaultAdminPort    = 8090
	DefaultPgVersion    = "16"
	DefaultMonthlyUSD   = 20
	DefaultPerKeyUSD    = 5
)

// NewManifest builds the manifest for a fresh project.
func NewManifest(opts Options, cliVersion, templateVersion string) Manifest {
	m := Manifest{
		Version:         ManifestVersion,
		GeneratedBy:     cliVersion,
		TemplateVersion: templateVersion,
		Options:         opts,
	}
	m.Services.App.Port = DefaultAppPort
	m.Services.App.AdminPort = DefaultAdminPort
	m.Services.Postgres.Version = DefaultPgVersion
	m.Budget.MonthlyUSD = DefaultMonthlyUSD
	m.Budget.PerKeyDefaultUSD = DefaultPerKeyUSD
	return m
}

// ManifestFileName is the marker file that makes a directory a llmberth project.
const ManifestFileName = ".llmberth.yaml"

// AdminPort satisfies admin.AdminPorter.
func (m Manifest) AdminPort() int { return m.Services.App.AdminPort }

// AppPort returns the public API port.
func (m Manifest) AppPort() int { return m.Services.App.Port }

// WriteManifest marshals m to path with 0600-ish safe defaults (it is not
// secret, but no reason to be world-writable either).
func WriteManifest(path string, m Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// LoadManifest reads and validates a manifest from path.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if m.Version != ManifestVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest version %d (want %d); regenerate or upgrade the project", m.Version, ManifestVersion)
	}
	return m, nil
}

// FindManifest walks up from dir looking for .llmberth.yaml, returning the
// project root dir. This is how CLI commands scope themselves to a project.
func FindManifest(dir string) (string, error) {
	cur := dir
	for {
		candidate := cur + string(os.PathSeparator) + ManifestFileName
		if _, err := os.Stat(candidate); err == nil {
			return cur, nil
		}
		parent := parentDir(cur)
		if parent == cur {
			return "", fmt.Errorf("no %s found in %s or any parent directory; run inside a llmberth project or use --path", ManifestFileName, dir)
		}
		cur = parent
	}
}

func parentDir(dir string) string {
	i := len(dir) - 1
	for i >= 0 && dir[i] != '/' {
		i--
	}
	if i <= 0 {
		return "/"
	}
	return dir[:i]
}
