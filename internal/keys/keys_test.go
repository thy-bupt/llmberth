package keys

import (
	"strings"
	"testing"
)

func TestNewAdminToken(t *testing.T) {
	a, err := NewAdminToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewAdminToken()
	if len(a) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(a))
	}
	if a == b {
		t.Fatal("two tokens identical; rand is broken")
	}
}

func TestNewAPIKey(t *testing.T) {
	full, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(full, APIKeyPrefix) {
		t.Fatalf("key %q missing prefix", full)
	}
	body := strings.TrimPrefix(full, APIKeyPrefix)
	if len(body) != 43 {
		t.Fatalf("body length = %d, want 43 (32 bytes base64url)", len(body))
	}
	if prefix != APIKeyPrefix+body[:8] {
		t.Fatalf("prefix %q != first 8 body chars", prefix)
	}
	if got := KeyPrefix(full); got != prefix {
		t.Fatalf("KeyPrefix(%q) = %q, want %q", full, got, prefix)
	}
}

func TestHashAndVerify(t *testing.T) {
	full, _, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	h := HashKey(full)
	if strings.Contains(h, full) || len(h) != 64 {
		t.Fatalf("hash %q malformed", h)
	}
	if !VerifyKey(full, h) {
		t.Fatal("VerifyKey rejected its own hash")
	}
	if VerifyKey(full+"x", h) {
		t.Fatal("VerifyKey accepted a tampered key")
	}
	// Hash must not leak the key.
	if strings.Contains(h, "lbt_live") {
		t.Fatal("hash contains plaintext marker")
	}
}
