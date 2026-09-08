package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thy-bupt/llmberth/internal/scaffold"
)

// SecurityCheckNames is the fixed twelve-item list (plan §5.7), in order.
// The doctor output maps 1:1 to this list so the audit is diffable.
var SecurityCheckNames = []string{
	".env not git-tracked",          // 1
	"no credential literals",        // 2
	"admin API loopback only",       // 3
	"postgres not published public", // 4
	"images pinned by digest",       // 5
	"non-root container user",       // 6
	"read_only rootfs + tmpfs",      // 7
	"prod profile TLS in place",     // 8
	"message logging off",           // 9
	"budget configured",             // 10
	"keys hashed at rest",           // 11
	"vuln scan gate configured",     // 12
}

// CheckSecurity runs the twelve security checks against a generated
// project directory. Like all doctor logic it is read-only.
func CheckSecurity(dir string) ([]Check, error) {
	checks := []Check{
		checkEnvNotTracked(dir),
		checkNoCredentialLiterals(dir),
		checkAdminLoopback(dir),
		checkPostgresNotPublic(dir),
		checkImageDigest(dir),
		checkNonRoot(dir),
		checkReadOnlyTmpfs(dir),
		checkProdTLS(dir),
		checkLogMessagesOff(dir),
		checkBudgetConfigured(dir),
		checkKeysHashedOnly(dir),
		checkVulnGateConfigured(dir),
	}
	return checks, nil
}

func okOrFail(name string, ok bool, detail string) Check {
	if ok {
		return Check{Name: name, Sev: Ok, Detail: detail}
	}
	return Check{Name: name, Sev: Fail, Detail: detail}
}

// 1 — .env must not be tracked by the project's git repo.
func checkEnvNotTracked(dir string) Check {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return okOrFail(SecurityCheckNames[0], false, "project has no .git — run git init first")
	}
	c := exec.Command("git", "ls-files", "--error-unmatch", "--", ".env") //nolint:gosec // constant argv
	c.Dir = dir
	err := c.Run()
	if err == nil {
		return okOrFail(SecurityCheckNames[0], false, ".env is tracked by git — remove it: git rm --cached .env")
	}
	return okOrFail(SecurityCheckNames[0], true, ".env is gitignored (git ls-files finds nothing)")
}

// credentialPatterns are the literal shapes a real credential would have.
// Placeholders (YOUR_API_KEY etc.) intentionally do not match.
var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),          // OpenAI-style
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),         // GitHub PAT
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{30,}`),       // Google API key
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),             // AWS access key id
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`), // Slack token
}

// 2 — no credential literals anywhere in the project sources.
func checkNoCredentialLiterals(dir string) Check {
	skipDirs := map[string]bool{".git": true, ".github": true, "bin": true, "static": true}
	found := ""
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // transient (git maintenance etc.) — skip
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if strings.HasSuffix(rel, ".env") || rel == ".llmberth-admin-token" {
			return nil
		}
		// Read-only scan of the operator's own project tree; the walk is the
		// scan itself, so the TOCTOU class gosec flags is the mechanism here
		// (no writes happen — worst case a symlink adds/omits one file).
		data, err := os.ReadFile(p) //nolint:gosec // read-only credential scan
		if err != nil {
			return nil
		}
		for _, re := range credentialPatterns {
			if m := re.Find(data); m != nil {
				found = fmt.Sprintf("%s:%s", rel, string(m))
				return filepath.SkipDir // stop at first hit
			}
		}
		return nil
	})
	if err != nil {
		return okOrFail(SecurityCheckNames[1], false, err.Error())
	}
	if found != "" {
		return okOrFail(SecurityCheckNames[1], false, "credential-shaped literal found: "+found)
	}
	return okOrFail(SecurityCheckNames[1], true, "no credential-shaped literals in sources")
}

// 3 — the admin port mapping must be loopback-only (dev) or absent (prod).
// Static: read the compose files, keep the check docker-free.
func checkAdminLoopback(dir string) Check {
	dev := readFile(dir, "docker-compose.yml")
	prod := readFile(dir, "docker-compose.prod.yml")
	// dev: admin 8090 must map 127.0.0.1:…:8090
	if strings.Contains(dev, "127.0.0.1:${ADMIN_PORT:-8090}:8090") || strings.Contains(dev, "127.0.0.1:"+defaultAdminPort+":8090") {
		// prod: admin must have no host mapping at all (loopback by construction)
		if strings.Contains(prod, "8090") && !strings.Contains(prod, "127.0.0.1:8090") && !strings.Contains(prod, "${ADMIN_PORT") {
			return okOrFail(SecurityCheckNames[2], false, "prod override mentions admin port without loopback binding")
		}
		return okOrFail(SecurityCheckNames[2], true, "admin published to 127.0.0.1 only (dev), no host mapping in prod")
	}
	return okOrFail(SecurityCheckNames[2], false, "admin port mapping not found or not loopback-bound in docker-compose.yml")
}

// 4 — postgres must never publish a public (0.0.0.0 / unbound) port.
func checkPostgresNotPublic(dir string) Check {
	dev := readFile(dir, "docker-compose.yml")
	prod := readFile(dir, "docker-compose.prod.yml")
	if strings.Contains(dev, "127.0.0.1:${POSTGRES_PORT:-15432}:5432") && !strings.Contains(prod, "5432") {
		return okOrFail(SecurityCheckNames[3], true, "postgres published to 127.0.0.1 only (dev), no host mapping in prod")
	}
	if strings.Contains(dev, "127.0.0.1:"+defaultPgPort+":5432") && !strings.Contains(prod, "5432") {
		return okOrFail(SecurityCheckNames[3], true, "postgres published to 127.0.0.1 only (dev), no host mapping in prod")
	}
	return okOrFail(SecurityCheckNames[3], false, "postgres port mapping unexpected (check docker-compose.yml / prod override)")
}

// 5 — base images pinned by digest in Dockerfile and compose.
func checkImageDigest(dir string) Check {
	dockerfile := readFile(dir, "Dockerfile")
	compose := readFile(dir, "docker-compose.yml")
	// Every FROM in the Dockerfile must carry @sha256:.
	froms := regexp.MustCompile(`(?m)^FROM\s+([^\s]+)`).FindAllStringSubmatch(dockerfile, -1)
	if len(froms) == 0 {
		return okOrFail(SecurityCheckNames[4], false, "Dockerfile has no FROM")
	}
	for _, f := range froms {
		if !strings.Contains(f[1], "@sha256:") {
			return okOrFail(SecurityCheckNames[4], false, "unpinned base image: "+f[1]+" (add @sha256:…)")
		}
	}
	if !strings.Contains(compose, "postgres:") || !strings.Contains(compose, "@sha256:") {
		return okOrFail(SecurityCheckNames[4], false, "compose postgres image not digest-pinned")
	}
	return okOrFail(SecurityCheckNames[4], true, "all base images pinned by digest")
}

// 6 — the container must run as a non-root user.
func checkNonRoot(dir string) Check {
	dockerfile := readFile(dir, "Dockerfile")
	for _, line := range strings.Split(dockerfile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "USER ") {
			user := strings.TrimPrefix(line, "USER ")
			if user != "root" && user != "0" {
				return okOrFail(SecurityCheckNames[5], true, "USER "+user)
			}
			return okOrFail(SecurityCheckNames[5], false, "USER "+user+" runs as root")
		}
	}
	return okOrFail(SecurityCheckNames[5], false, "Dockerfile has no USER directive")
}

// 7 — app container read_only rootfs with a tmpfs scratch.
func checkReadOnlyTmpfs(dir string) Check {
	compose := readFile(dir, "docker-compose.yml")
	if !strings.Contains(compose, "read_only: true") {
		return okOrFail(SecurityCheckNames[6], false, "app service missing read_only: true")
	}
	if !strings.Contains(compose, "tmpfs") {
		return okOrFail(SecurityCheckNames[6], false, "app service missing tmpfs scratch")
	}
	return okOrFail(SecurityCheckNames[6], true, "read_only: true + tmpfs in place")
}

// 8 — prod profile present with Caddy TLS (443) and Caddyfile.
func checkProdTLS(dir string) Check {
	prod := readFile(dir, "docker-compose.prod.yml")
	caddyfile := readFile(dir, "Caddyfile")
	switch {
	case !strings.Contains(prod, "caddy:"):
		return okOrFail(SecurityCheckNames[7], false, "docker-compose.prod.yml missing caddy service")
	case !strings.Contains(prod, `"443:443"`):
		return okOrFail(SecurityCheckNames[7], false, "caddy does not publish 443")
	case !strings.Contains(caddyfile, "reverse_proxy"):
		return okOrFail(SecurityCheckNames[7], false, "Caddyfile missing reverse_proxy")
	default:
		return okOrFail(SecurityCheckNames[7], true, "prod Caddy TLS in place (443 + Caddyfile)")
	}
}

// 9 — message body logging must be off (LOG_MESSAGES unset or 0).
func checkLogMessagesOff(dir string) Check {
	env := readFile(dir, ".env")
	for _, line := range strings.Split(env, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "LOG_MESSAGES=") {
			v := strings.TrimPrefix(line, "LOG_MESSAGES=")
			if v == "0" || v == "false" || v == "" {
				return okOrFail(SecurityCheckNames[8], true, "LOG_MESSAGES="+v)
			}
			return okOrFail(SecurityCheckNames[8], false, "LOG_MESSAGES="+v+" — message bodies would be persisted")
		}
	}
	return okOrFail(SecurityCheckNames[8], true, "LOG_MESSAGES unset (defaults to off)")
}

// 10 — a monthly budget must be configured in the manifest.
func checkBudgetConfigured(dir string) Check {
	m, err := LoadManifestAt(dir)
	if err != nil {
		return okOrFail(SecurityCheckNames[9], false, err.Error())
	}
	if m.Budget.MonthlyUSD > 0 {
		return okOrFail(SecurityCheckNames[9], true, fmt.Sprintf("monthly budget $%.2f", m.Budget.MonthlyUSD))
	}
	return okOrFail(SecurityCheckNames[9], false, "monthly_usd is 0 — budgeting disabled")
}

// 11 — the api_keys table stores hash + prefix, never the full key.
func checkKeysHashedOnly(dir string) Check {
	migration, err := os.ReadFile(findMigrationUp(dir))
	if err != nil {
		return okOrFail(SecurityCheckNames[10], false, err.Error())
	}
	text := string(migration)
	table := text
	if i := strings.Index(text, "CREATE TABLE IF NOT EXISTS api_keys"); i >= 0 {
		rest := text[i:]
		if j := strings.Index(rest, ");"); j >= 0 {
			table = rest[:j]
		}
	}
	switch {
	case !strings.Contains(table, "key_hash"):
		return okOrFail(SecurityCheckNames[10], false, "api_keys has no key_hash column")
	case strings.Contains(table, "key text") || strings.Contains(table, "full_key"):
		return okOrFail(SecurityCheckNames[10], false, "api_keys stores a full key column")
	default:
		return okOrFail(SecurityCheckNames[10], true, "api_keys stores key_hash + prefix only")
	}
}

// 12 — a vulnerability gate must be configured (CI govulncheck).
func checkVulnGateConfigured(dir string) Check {
	ci := readFile(dir, ".github/workflows/ci.yml")
	// The generated project's CI runs govulncheck; also accept a Makefile
	// target as a manual gate.
	if strings.Contains(ci, "govulncheck") {
		return okOrFail(SecurityCheckNames[11], true, "CI runs govulncheck")
	}
	makefile := readFile(dir, "Makefile")
	if strings.Contains(makefile, "govulncheck") {
		return okOrFail(SecurityCheckNames[11], true, "Makefile has a govulncheck target")
	}
	return okOrFail(SecurityCheckNames[11], false, "no govulncheck gate in ci.yml or Makefile")
}

// readFile reads a project file; missing files read as "".
func readFile(dir, name string) string {
	// readFile reads a fixed project-relative file (name is a compile-time
	// constant at every call site); the join target is the operator's own
	// project directory, not user input.
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // fixed project-relative names
	if err != nil {
		return ""
	}
	return string(data)
}

// findMigrationUp locates the up migration file (internal/store/migrations).
func findMigrationUp(dir string) string {
	root := filepath.Join(dir, "internal", "store", "migrations")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			// e.Name() comes from ReadDir of the project's own migrations
			// directory — not user input.
			return filepath.Join(root, e.Name()) //nolint:gosec // controlled dir listing
		}
	}
	return ""
}

// LoadManifestAt reads the manifest from a project directory (doctor-side).
func LoadManifestAt(dir string) (scaffold.Manifest, error) {
	return scaffold.LoadManifest(filepath.Join(dir, scaffold.ManifestFileName))
}

// Port constants mirrored from the template's compose defaults (kept local
// so the checks stay self-contained).
const (
	defaultAdminPort = "8090"
	defaultPgPort    = "15432"
)

// ScanFileForCredentials feeds file bytes into the credential scanner;
// used by tests (and future scans of arbitrary files).
func ScanFileForCredentials(data []byte) string {
	for _, re := range credentialPatterns {
		if m := re.Find(data); m != nil {
			return string(m)
		}
	}
	return ""
}
