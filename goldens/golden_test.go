//go:build golden

// Golden generation test (plan §7): the anti-template-rot gate. CI generates
// the v0.1 golden matrix combination (openai + postgres + single-page UI)
// through the real `llmberth init` binary, then:
//
//  1. file-snapshot diff of every generated, secret-free file (goldens/)
//  2. go build + go test of the generated project (fake provider, no network)
//  3. docker compose config -q (stack must be syntactically valid)
//
// First run (or after an intentional template change):
//
//	go test -tags golden -run TestGolden ./goldens/... -update
package goldens

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden snapshot")

const combination = "openai_postgres_ui"

// snapshot excludes secret-bearing and environment-dependent files: .env and
// .llmberth-admin-token carry random tokens by design; .git is not a file
// snapshot concern.
func excluded(rel string) bool {
	switch rel {
	case ".env", ".llmberth-admin-token":
		return true
	}
	return strings.HasPrefix(rel, ".git/") || rel == ".git"
}

// Every exec.Command in this file takes compile-time constant arguments
// only; the varying inputs (repo root, generated project dir) enter
// exclusively via cmd.Dir, never argv.
func buildCLI(t *testing.T, repoRoot string) {
	t.Helper()
	c := exec.Command("go", "build", "-o", "/tmp/llmberth-golden", "./cmd/llmberth")
	c.Dir = repoRoot
	c.Env = append(os.Environ(), "CGO_ENABLED=0")
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, outB)
	}
}

func runInit(t *testing.T, workDir string) {
	t.Helper()
	c := exec.Command("/tmp/llmberth-golden", "init", "app", "--provider=openai", "--ui=single-page", "--module=github.com/golden/app")
	c.Dir = workDir
	c.Env = os.Environ()
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("llmberth init failed: %v\n%s", err, outB)
	}
}

func runGoModTidy(t *testing.T, projDir string) {
	t.Helper()
	c := exec.Command("go", "mod", "tidy")
	c.Dir = projDir
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, outB)
	}
}

func runGoBuildAll(t *testing.T, projDir string) {
	t.Helper()
	c := exec.Command("go", "build", "./...")
	c.Dir = projDir
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, outB)
	}
}

func runGoTestAll(t *testing.T, projDir string) {
	t.Helper()
	c := exec.Command("go", "test", "./...")
	c.Dir = projDir
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go test failed: %v\n%s", err, outB)
	}
}

func runComposeConfig(t *testing.T, projDir string) {
	t.Helper()
	c := exec.Command("docker", "compose", "config", "-q")
	c.Dir = projDir
	if outB, err := c.CombinedOutput(); err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, outB)
	}
}

func TestGolden(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain required for golden test")
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Build the CLI from this very checkout (tests the real binary path).
	buildCLI(t, repoRoot)

	// 2. Generate the golden combination via the real init command.
	work := t.TempDir()
	runInit(t, work)
	proj := filepath.Join(work, "app")

	// 3. File snapshot diff (secret-free files only).
	type fileHash struct {
		Path string `json:"path"`
		Sha  string `json:"sha256"`
	}
	var files []fileHash
	err = filepath.Walk(proj, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(proj, p)
		if err != nil {
			return err
		}
		if excluded(rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, fileHash{Path: rel, Sha: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(files) < 20 {
		t.Fatalf("suspiciously few generated files: %d", len(files))
	}

	snapshotPath := filepath.Join(repoRoot, "goldens", combination, "snapshot.json")
	got, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(snapshotPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("snapshot updated: %d files", len(files))
	} else {
		want, err := os.ReadFile(snapshotPath)
		if err != nil {
			t.Fatalf("golden snapshot missing; run: go test -tags golden -run TestGolden ./goldens/... -update (%v)", err)
		}
		if string(want) != string(got) {
			var wantFiles, gotFiles []fileHash
			_ = json.Unmarshal(want, &wantFiles)
			_ = json.Unmarshal(got, &gotFiles)
			gotMap := map[string]string{}
			for _, f := range gotFiles {
				gotMap[f.Path] = f.Sha
			}
			wantMap := map[string]string{}
			for _, f := range wantFiles {
				wantMap[f.Path] = f.Sha
			}
			changed := 0
			for _, f := range gotFiles {
				if wantMap[f.Path] != f.Sha {
					t.Errorf("template drift in %q (sha changed)", f.Path)
					if changed++; changed >= 5 {
						break
					}
				}
			}
			// Missing detection: a file present in the snapshot but absent
			// from the generated tree — e.g. silently dropped by .gitignore
			// and lost on a fresh clone.
			missed := 0
			for _, f := range wantFiles {
				if _, exists := gotMap[f.Path]; !exists {
					t.Errorf("file %q is in the snapshot but NOT generated (missing from template?)", f.Path)
					if missed++; missed >= 5 {
						break
					}
				}
			}
			t.Fatal("golden snapshot mismatch — if intentional, re-run with -update")
		}
	}

	// 4. The generated project must build and pass its own tests
	// (fake provider: no network, no real keys).
	runGoModTidy(t, proj)
	runGoBuildAll(t, proj)
	runGoTestAll(t, proj)

	// 5. The stack must be compose-valid.
	if _, err := exec.LookPath("docker"); err == nil {
		runComposeConfig(t, proj)
	} else {
		t.Log("docker not found; skipped compose config")
	}

	fmt.Printf("golden combination %s: %d files verified\n", combination, len(files))
}
