package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/THY17308111153/llmberth/internal/scaffold"
)

// writeTestProject creates a minimal project skeleton for doctor tests.
func writeTestProject(t *testing.T, envContent, tokenContent string) string {
	t.Helper()
	dir := t.TempDir()
	m := scaffold.NewManifest(scaffold.Options{Name: "t", Module: "t", Provider: "fake", UI: "none", DB: "postgres"}, "dev", "dev")
	if err := scaffold.WriteManifest(filepath.Join(dir, scaffold.ManifestFileName), m); err != nil {
		t.Fatal(err)
	}
	if envContent != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if tokenContent != "" {
		if err := os.WriteFile(filepath.Join(dir, ".llmberth-admin-token"), []byte(tokenContent+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func fullEnv() string {
	return "POSTGRES_PASSWORD=x\nDATABASE_URL=postgres://x\nADMIN_TOKEN=tok123\nLLM_PROVIDER=fake\nLLM_MODEL=fake-chat\n"
}

func finding(res DoctorResult, name string) (Check, bool) {
	for _, c := range res.Checks {
		if strings.Contains(c.Name, name) {
			return c, true
		}
	}
	return Check{}, false
}

func TestDoctorReportsMissingEnvKeys(t *testing.T) {
	dir := writeTestProject(t, "POSTGRES_PASSWORD=x\nADMIN_TOKEN=tok123\n", "tok123")
	res, err := RunDoctor(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := finding(res, ".env")
	if !ok || c.Sev != Fail {
		t.Fatalf("expected .env failure, got %+v", c)
	}
	if !strings.Contains(c.Detail, "DATABASE_URL") || !strings.Contains(c.Detail, "LLM_PROVIDER") {
		t.Fatalf("missing keys not reported: %q", c.Detail)
	}
	if !res.HasFail() {
		t.Fatal("doctor must fail when .env is incomplete")
	}
}

func TestDoctorPassesOnCompleteProject(t *testing.T) {
	dir := writeTestProject(t, fullEnv(), "tok123")
	res, err := RunDoctor(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := finding(res, ".env"); !ok || c.Sev != Ok {
		t.Fatalf("complete .env should pass, got %+v", c)
	}
	if c, ok := finding(res, "admin token file"); !ok || c.Sev != Ok {
		t.Fatalf("matching tokens should pass, got %+v", c)
	}
	// Stack not running → reachability is a warning, not a failure.
	if _, running := finding(res, "reachable"); !running {
		t.Fatal("expected a reachability check entry")
	}
	if res.HasFail() {
		t.Fatalf("no docker/network-independent failure expected locally; got %+v", res.Checks)
	}
}

func TestDoctorFlagsTokenMismatch(t *testing.T) {
	dir := writeTestProject(t, fullEnv(), "DIFFERENT")
	res, err := RunDoctor(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := finding(res, "admin token file")
	if !ok || c.Sev != Warn {
		t.Fatalf("token mismatch should warn, got %+v", c)
	}
}

func TestDoctorMissingProject(t *testing.T) {
	res, err := RunDoctor(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := finding(res, "project manifest"); !ok || c.Sev != Fail {
		t.Fatalf("missing manifest must fail, got %+v", c)
	}
	if c, ok := finding(res, "admin token file"); !ok || c.Sev != Fail {
		t.Fatalf("missing token file must fail, got %+v", c)
	}
}
