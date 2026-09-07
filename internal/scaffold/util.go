package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// ensureEmptyDir verifies targetDir is absent or empty, creating it when
// absent. With force=true a non-empty directory is accepted as-is (files may
// be overwritten during generation).
func ensureEmptyDir(targetDir string, force bool) error {
	info, err := os.Stat(targetDir)
	if os.IsNotExist(err) {
		return os.MkdirAll(targetDir, 0o750)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", targetDir)
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("target directory %s is not empty (use --force to overwrite)", targetDir)
	}
	return nil
}

func srcToText(fsys fs.FS, p string) string {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		// Walk already opened the file successfully; a read failure here is
		// unrecoverable.
		panic(fmt.Sprintf("scaffold: read %s: %v", p, err))
	}
	return string(data)
}

func renderToFile(tmpl *template.Template, data tmplData, absOut string) error { //nolint:revive // tmplData is engine-internal
	if err := os.MkdirAll(filepath.Dir(absOut), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(absOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(absOut))
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func copyToFile(fsys fs.FS, src, absOut string) error {
	data, err := fs.ReadFile(fsys, src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(absOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(absOut))
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// fileMode picks output permissions: executables stay executable, everything
// else is 0644.
func fileMode(p string) os.FileMode {
	base := strings.ToLower(filepath.Base(p))
	if strings.HasSuffix(base, ".sh") || base == "air" {
		return 0o755
	}
	return 0o644
}
