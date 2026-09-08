package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileKeychainRoundtrip exercises the AES-GCM degradation path end to
// end: set/get/delete survive reloads and the plaintext never hits disk.
func TestFileKeychainRoundtrip(t *testing.T) {
	t.Setenv(MasterKeyEnv, strings.Repeat("m", 40))
	dir := t.TempDir()

	fk, err := NewFileKeychain(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fk.Set("OPENAI_API_KEY", "sk-test-abc"); err != nil {
		t.Fatal(err)
	}
	got, err := fk.Get("OPENAI_API_KEY")
	if err != nil || got != "sk-test-abc" {
		t.Fatalf("get: %q %v", got, err)
	}

	// Reload from disk: a fresh instance must still decrypt.
	fk2, err := NewFileKeychain(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = fk2.Get("OPENAI_API_KEY")
	if err != nil || got != "sk-test-abc" {
		t.Fatalf("get after reload: %q %v", got, err)
	}

	// Plaintext never on disk.
	data, err := os.ReadFile(filepath.Join(dir, "secrets.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-test-abc") {
		t.Fatal("plaintext leaked into the encrypted file")
	}

	// Wrong master key must fail closed.
	t.Setenv(MasterKeyEnv, strings.Repeat("x", 40))
	if _, err := NewFileKeychain(dir); err == nil {
		t.Fatal("wrong master key must fail to decrypt")
	}
}

func TestFileKeychainRequiresMasterKey(t *testing.T) {
	t.Setenv(MasterKeyEnv, "")
	if _, err := NewFileKeychain(t.TempDir()); err == nil {
		t.Fatal("missing master key must fail closed")
	}
	t.Setenv(MasterKeyEnv, "short")
	if _, err := NewFileKeychain(t.TempDir()); err == nil {
		t.Fatal("short master key must fail closed")
	}
}

func TestFileKeychainDelete(t *testing.T) {
	t.Setenv(MasterKeyEnv, strings.Repeat("m", 40))
	fk, err := NewFileKeychain(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = fk.Set("DEEPSEEK_API_KEY", "v")
	if err := fk.Delete("DEEPSEEK_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := fk.Get("DEEPSEEK_API_KEY"); err == nil {
		t.Fatal("deleted key still readable")
	}
}

func TestOverlayPicksStoredKeys(t *testing.T) {
	t.Setenv(MasterKeyEnv, strings.Repeat("m", 40))
	fk, err := NewFileKeychain(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = fk.Set("OPENAI_API_KEY", "sk-stored")
	overlay, err := Overlay(fk)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range overlay {
		if kv == "OPENAI_API_KEY=sk-stored" {
			found = true
		}
		if strings.Contains(kv, "sk-stored") && !found {
			t.Fatalf("unexpected key in overlay: %s", kv)
		}
	}
	if !found {
		t.Fatal("stored OPENAI_API_KEY missing from overlay")
	}
}

func TestIsUpstreamEnv(t *testing.T) {
	if !IsUpstreamEnv("openai_api_key") || !IsUpstreamEnv("DASHSCOPE_API_KEY") {
		t.Fatal("known env names must be recognized")
	}
	if IsUpstreamEnv("ADMIN_TOKEN") || IsUpstreamEnv("") {
		t.Fatal("non-upstream names must not be recognized")
	}
}
