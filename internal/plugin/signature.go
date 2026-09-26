// Package plugin provides the plugin loading and execution subsystem.
// This file implements optional cryptographic signature verification for plugins.
package plugin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// SignatureVerifier validates plugin signatures using ECDSA.
// When enabled, plugins must carry a .sig file alongside their binary.
type SignatureVerifier struct {
	enabled     bool
	publicKey   *ecdsa.PublicKey
	allowedKeys map[string]*ecdsa.PublicKey
}

// SignatureVerifierConfig controls signature verification behavior.
type SignatureVerifierConfig struct {
	Enabled    bool
	PublicKey  string
	PublicKeys map[string]string
	Required   bool
}

// DefaultSignatureVerifierConfig returns defaults: disabled until user enables.
func DefaultSignatureVerifierConfig() *SignatureVerifierConfig {
	return &SignatureVerifierConfig{
		Enabled:  false,
		Required: true,
	}
}

// LoadSignatureVerifier reads the config and returns a ready-to-use verifier.
func LoadSignatureVerifier(cfg *SignatureVerifierConfig) (*SignatureVerifier, error) {
	if cfg == nil {
		return &SignatureVerifier{enabled: false}, nil
	}

	v := &SignatureVerifier{
		enabled:     cfg.Enabled,
		allowedKeys: make(map[string]*ecdsa.PublicKey),
	}

	if !cfg.Enabled {
		return v, nil
	}

	if cfg.PublicKey != "" {
		key, err := parsePEMPublicKey([]byte(cfg.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("signature verifier: parse public key: %w", err)
		}
		v.publicKey = key
		v.allowedKeys["default"] = key
	}

	for id, pemData := range cfg.PublicKeys {
		key, err := parsePEMPublicKey([]byte(pemData))
		if err != nil {
			return nil, fmt.Errorf("signature verifier: parse public key %q: %w", id, err)
		}
		v.allowedKeys[id] = key
	}

	return v, nil
}

// VerifyPlugin checks that the binary at path is signed by an allowed key.
func (v *SignatureVerifier) VerifyPlugin(path string) error {
	if !v.enabled {
		return nil
	}

	sigPath := path + ".sig"
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("plugin %q: missing signature file %q", path, sigPath)
		}
		return fmt.Errorf("plugin %q: read signature: %w", path, err)
	}

	binData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("plugin %q: read binary: %w", path, err)
	}

	signature, err := base64.StdEncoding.DecodeString(string(sigData))
	if err != nil {
		return fmt.Errorf("plugin %q: decode signature: %w", path, err)
	}

	h := sha256.Sum256(binData)
	ok := false

	if v.publicKey != nil {
		ok = ecdsa.VerifyASN1(v.publicKey, h[:], signature)
	}

	if !ok {
		for _, key := range v.allowedKeys {
			if ecdsa.VerifyASN1(key, h[:], signature) {
				ok = true
				break
			}
		}
	}

	if !ok {
		return fmt.Errorf("plugin %q: signature verification failed", path)
	}

	return nil
}

func parsePEMPublicKey(pemData []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("no PEM data found")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX public key: %w", err)
	}

	eccPubKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not ECDSA")
	}

	return eccPubKey, nil
}

// GenerateKeyPair creates a new ECDSA key pair for testing.
func GenerateKeyPair() (*ecdsa.PrivateKey, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, "", err
	}

	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	return priv, string(pem.EncodeToMemory(pemBlock)), nil
}
