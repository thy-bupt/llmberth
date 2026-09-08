// Package keys holds keychain access for upstream provider API keys (plan
// §5.1): the OS keychain when available (macOS Keychain / Windows
// Credential Manager / secret-service), otherwise an AES-GCM encrypted
// file. Values never appear in logs or on disk in plaintext; the CLI only
// overlays them into subprocess environments (dev/up).
package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/99designs/keyring"
)

// KeychainService is the keyring service name used for upstream keys.
const KeychainService = "llmberth"

// Keychain is the upstream-key store interface.
type Keychain interface {
	Set(name, value string) error
	Get(name string) (string, error)
	Delete(name string) error
	List() ([]string, error)
}

// MasterKeyEnv is the env var holding the fallback-file encryption key
// (32+ chars). Only needed when no OS keychain backend is available.
const MasterKeyEnv = "LLMBERTH_MASTER_KEY"

// OpenKeyring returns the OS-backed keyring, or nil when unavailable (linux
// without secret-service etc.). Nil means callers must use the file backend.
func OpenKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName:              KeychainService,
		AllowedBackends:          []keyring.BackendType{keyring.KeychainBackend, keyring.WinCredBackend, keyring.SecretServiceBackend},
		KeychainTrustApplication: true,
	})
}

// OSKeychain adapts a keyring.Keyring to the Keychain interface.
type OSKeychain struct{ kr keyring.Keyring }

// Set stores a value (Keychain interface).
func (k OSKeychain) Set(name, value string) error {
	return k.kr.Set(keyring.Item{Key: name, Data: []byte(value)})
}

// Get returns a stored value (Keychain interface).
func (k OSKeychain) Get(name string) (string, error) {
	item, err := k.kr.Get(name)
	if err != nil {
		return "", err
	}
	return string(item.Data), nil
}

// Delete removes a stored value (Keychain interface).
func (k OSKeychain) Delete(name string) error { return k.kr.Remove(name) }

// List returns stored names (Keychain interface; values never returned).
func (k OSKeychain) List() ([]string, error) { return k.kr.Keys() }

// FileKeychain is the AES-GCM encrypted fallback: a JSON map encrypted with
// the LLMBERTH_MASTER_KEY env var (plan §5.1 degradation path).
type FileKeychain struct {
	path   string
	master []byte
	mu     sync.Mutex
	cached map[string]string
}

// NewFileKeychain opens (and lazily creates) the encrypted file backend in
// dir (default ~/.llmberth). The master key must be set via MasterKeyEnv.
func NewFileKeychain(dir string) (*FileKeychain, error) {
	master := os.Getenv(MasterKeyEnv)
	if len(master) < 32 {
		return nil, fmt.Errorf("%s is required for the file keychain (32+ chars); export it or run on an OS keychain backend", MasterKeyEnv)
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".llmberth")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	fk := &FileKeychain{
		path:   filepath.Join(dir, "secrets.enc"),
		master: []byte(master),
		cached: map[string]string{},
	}
	if err := fk.load(); err != nil {
		return nil, err
	}
	return fk, nil
}

func (f *FileKeychain) load() error {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := f.decrypt(data)
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", f.path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	f.cached = m
	return nil
}

func (f *FileKeychain) save() error {
	raw, err := json.Marshal(f.cached)
	if err != nil {
		return err
	}
	enc, err := f.encrypt(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, enc, 0o600)
}

func (f *FileKeychain) encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(f.master[:32])
	if err != nil {
		return nil, err
	}
	aed, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aed.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aed.Seal(nonce, nonce, plain, nil), nil
}

func (f *FileKeychain) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(f.master[:32])
	if err != nil {
		return nil, err
	}
	aed, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < aed.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := data[:aed.NonceSize()], data[aed.NonceSize():]
	return aed.Open(nil, nonce, ct, nil)
}

// Set stores a value (Keychain interface).
func (f *FileKeychain) Set(name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached[name] = value
	return f.save()
}

// Get returns a stored value (Keychain interface).
func (f *FileKeychain) Get(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.cached[name]
	if !ok {
		return "", fmt.Errorf("no upstream key stored under %q", name)
	}
	return v, nil
}

// Delete removes a stored value (Keychain interface).
func (f *FileKeychain) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cached, name)
	return f.save()
}

// List returns stored names (Keychain interface; values never returned).
func (f *FileKeychain) List() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.cached))
	for k := range f.cached {
		out = append(out, k)
	}
	return out, nil
}

// OpenKeychain resolves the best available backend: OS keychain first, file
// fallback otherwise.
func OpenKeychain(dir string) (Keychain, error) {
	if kr, err := OpenKeyring(); err == nil {
		return OSKeychain{kr: kr}, nil
	}
	return NewFileKeychain(dir)
}

// envNames are the upstream key env vars the CLI knows how to overlay.
var envNames = []string{"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "DASHSCOPE_API_KEY", "UPSTREAM_API_KEY"}

// IsUpstreamEnv reports whether name is an overlayable upstream key env var.
func IsUpstreamEnv(name string) bool {
	for _, n := range envNames {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// UpstreamEnvNames lists the overlayable names (for prompts/help).
func UpstreamEnvNames() []string { return append([]string(nil), envNames...) }

// Overlay returns KEY=VALUE pairs for every upstream env name present in the
// keychain; the CLI merges these over the project .env map so stored keys
// win without touching .env (plan §5.1: no plaintext persistence).
func Overlay(kc Keychain) ([]string, error) {
	var out []string
	for _, n := range envNames {
		if v, err := kc.Get(n); err == nil {
			out = append(out, n+"="+v)
		}
	}
	return out, nil
}
