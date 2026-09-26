package plugin

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSignatureVerifierDisabledByDefault(t *testing.T) {
	v, err := LoadSignatureVerifier(DefaultSignatureVerifierConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.enabled {
		t.Error("expected verifier to be disabled by default")
	}
}

func TestSignatureVerifierEnabled(t *testing.T) {
	cfg := &SignatureVerifierConfig{
		Enabled: true,
	}
	v, err := LoadSignatureVerifier(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.enabled {
		t.Error("expected verifier to be enabled")
	}
}

func TestSignatureVerification(t *testing.T) {
	// Generate key pair
	priv, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	// Create test binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test-plugin")
	err = os.WriteFile(binPath, []byte("test binary content"), 0o755)
	if err != nil {
		t.Fatalf("failed to write test binary: %v", err)
	}

	// Sign the binary
	h := sha256.Sum256([]byte("test binary content"))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Write signature file
	sigPath := binPath + ".sig"
	err = os.WriteFile(sigPath, []byte(base64.StdEncoding.EncodeToString(sig)), 0o644)
	if err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Test with valid config (no public key set yet)
	v := &SignatureVerifier{enabled: true}
	
	// Should fail without public key
	err = v.VerifyPlugin(binPath)
	if err == nil {
		t.Error("expected verification to fail without public key")
	}

	// Now add the public key
	pubKey := &priv.PublicKey
	v.publicKey = pubKey
	v.allowedKeys = map[string]*ecdsa.PublicKey{"default": pubKey}

	// Should pass with correct signature
	err = v.VerifyPlugin(binPath)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	// Test with tampered binary
	err = os.WriteFile(binPath, []byte("tampered content"), 0o755)
	if err != nil {
		t.Fatalf("failed to write tampered content: %v", err)
	}

	err = v.VerifyPlugin(binPath)
	if err == nil {
		t.Error("expected verification to fail for tampered binary")
	}
}

func TestSignatureMissing(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test-plugin")
	err := os.WriteFile(binPath, []byte("content"), 0o755)
	if err != nil {
		t.Fatalf("failed to write binary: %v", err)
	}

	v := &SignatureVerifier{enabled: true}
	err = v.VerifyPlugin(binPath)
	if err == nil {
		t.Error("expected error for missing signature")
	}
}
