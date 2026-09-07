package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"app/go.mod.tmpl":                        &fstest.MapFile{Data: []byte("module {{.Module}}\n\ngo 1.25\n")},
		"app/README.md.tmpl":                     &fstest.MapFile{Data: []byte("# {{.Name}} ({{.Provider.DisplayName}})\n")},
		"app/env-only.txt":                       &fstest.MapFile{Data: []byte("static\n")},
		"app/__ui__/static/index.html.tmpl":      &fstest.MapFile{Data: []byte("<h1>{{.Name}}</h1>\n")},
		"app/internal/api/server.go.tmpl":        &fstest.MapFile{Data: []byte("package api\n")},
		"app/.env.tmpl":                          &fstest.MapFile{Data: []byte("ADMIN_TOKEN={{.AdminToken}}\n")},
		"app/.llmberth.yaml.tmpl":                &fstest.MapFile{Data: []byte("version: {{.Manifest.Version}}\ntemplate_version: {{.Manifest.TemplateVersion}}\n")},
	}
}

func baseOpts() Options {
	return Options{Name: "demo", Module: "example.com/demo", Provider: "openai", UI: "none", DB: "postgres"}
}

func TestGenerateWithoutUI(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{FS: testFS(), Root: "app", TemplateVersion: "0.1.0", CLIVersion: "0.1.0"}
	files, manifest, err := e.Generate(baseOpts(), dir, false, "tok-123", "dbpw-123")
	if err != nil {
		t.Fatal(err)
	}

	rel := map[string]bool{}
	for _, f := range files {
		rel[f.RelPath] = true
	}
	// Conditional dir must be stripped of output when UI=none.
	if rel["static/index.html"] {
		t.Fatal("__ui__ content rendered although UI=none")
	}
	for _, want := range []string{"go.mod", "README.md", "env-only.txt", "internal/api/server.go", ".env", ".llmberth.yaml"} {
		if !rel[want] {
			t.Errorf("missing output file %q; got %v", want, rel)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gomod) != "module example.com/demo\n\ngo 1.25\n" {
		t.Errorf("go.mod rendered as %q", gomod)
	}

	env, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(env) != "ADMIN_TOKEN=tok-123\n" {
		t.Errorf(".env rendered as %q", env)
	}

	if manifest.TemplateVersion != "0.1.0" || manifest.Options != baseOpts() {
		t.Errorf("manifest stamped wrong: %+v", manifest)
	}
}

func TestGenerateWithUI(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{FS: testFS(), Root: "app", TemplateVersion: "0.1.0", CLIVersion: "0.1.0"}
	opts := baseOpts()
	opts.UI = "single-page"
	files, _, err := e.Generate(opts, dir, false, "tok", "dbpw")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f.RelPath == "static/index.html" {
			found = true
		}
	}
	if !found {
		t.Fatal("__ui__ content not emitted with UI=single-page")
	}
}

func TestGenerateRejectsNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Engine{FS: testFS(), Root: "app", TemplateVersion: "0.1.0", CLIVersion: "0.1.0"}
	if _, _, err := e.Generate(baseOpts(), dir, false, "tok", "dbpw"); err == nil {
		t.Fatal("expected error generating into non-empty dir without --force")
	}
	if _, _, err := e.Generate(baseOpts(), dir, true, "tok", "dbpw"); err != nil {
		t.Fatalf("expected force to succeed, got %v", err)
	}
}

func TestGenerateValidatesOptions(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{FS: testFS(), Root: "app", TemplateVersion: "0.1.0", CLIVersion: "0.1.0"}
	bad := baseOpts()
	bad.Provider = "anthropic" // not in v0.1 presets
	if _, _, err := e.Generate(bad, dir, false, "tok", "dbpw"); err == nil {
		t.Fatal("expected validation error for unknown provider")
	}
}

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManifest(baseOpts(), "0.1.0", "0.1.0")
	path := filepath.Join(dir, ManifestFileName)
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Options.Name != "demo" || got.Services.App.Port != DefaultAppPort || got.Budget.MonthlyUSD != DefaultMonthlyUSD {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if err := os.WriteFile(path, []byte("version: 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected unsupported-version error")
	}
}

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		mutate func(*Options)
		wantOK bool
	}{
		{func(_ *Options) {}, true},
		{func(o *Options) { o.Name = "Bad Name" }, false},
		{func(o *Options) { o.Name = "9lead" }, false},
		{func(o *Options) { o.Provider = "azure" }, false},
		{func(o *Options) { o.UI = "vue" }, false},
		{func(o *Options) { o.DB = "sqlite" }, false},
		{func(o *Options) { o.Module = "" }, false},
	}
	for i, tc := range cases {
		o := baseOpts()
		tc.mutate(&o)
		err := o.Validate()
		if tc.wantOK && err != nil {
			t.Errorf("case %d: unexpected error %v", i, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
