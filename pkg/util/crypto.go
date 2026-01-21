package util

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
)

// jwkKey represents an Ed25519 JWK key
type jwkKey struct {
	Kty string `json:"kty"` // Key type (should be "OKP")
	Crv string `json:"crv"` // Curve (should be "Ed25519")
	D   string `json:"d"`   // Private key (base64url encoded)
	X   string `json:"x"`   // Public key (base64url encoded)
}

// ValidatePEMKey validates that a PEM-encoded string contains a valid Ed25519 private key
func ValidatePEMKey(pemData string) error {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	if block.Type != "PRIVATE KEY" {
		return fmt.Errorf("invalid PEM type: expected PRIVATE KEY, got %s", block.Type)
	}

	// Validate that it's actually an Ed25519 key
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
	}

	if _, ok := privateKey.(ed25519.PrivateKey); !ok {
		return fmt.Errorf("invalid key type: expected Ed25519 private key, got %T", privateKey)
	}

	return nil
}

// IsPEMKey checks if the input string is in PEM format
func IsPEMKey(data string) bool {
	block, _ := pem.Decode([]byte(data))
	return block != nil
}

// ConvertJWKToPEM converts an Ed25519 JWK to PEM format
func ConvertJWKToPEM(jwkJSON string) ([]byte, error) {
	// Parse as JWK
	var key jwkKey
	if err := json.Unmarshal([]byte(jwkJSON), &key); err != nil {
		return nil, fmt.Errorf("failed to parse JWK: %w", err)
	}

	if key.Kty != "OKP" || key.Crv != "Ed25519" {
		return nil, fmt.Errorf("invalid key type: expected OKP/Ed25519, got %s/%s", key.Kty, key.Crv)
	}

	// Decode private key from base64url
	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(key.D)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	if len(privateKeyBytes) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid private key size: expected %d bytes, got %d", ed25519.SeedSize, len(privateKeyBytes))
	}

	// Generate Ed25519 private key from seed
	privateKey := ed25519.NewKeyFromSeed(privateKeyBytes)

	// Marshal to PKCS#8 format using stdlib
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PKCS#8: %w", err)
	}

	// Encode to PEM
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	}

	return pem.EncodeToMemory(pemBlock), nil
}

// ConvertPEMToJWK converts an Ed25519 PEM private key to JWK format
func ConvertPEMToJWK(pemData string) (string, error) {
	// Decode PEM block
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}

	if block.Type != "PRIVATE KEY" {
		return "", fmt.Errorf("invalid PEM type: expected PRIVATE KEY, got %s", block.Type)
	}

	// Parse PKCS#8 private key
	privateKeyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
	}

	// Ensure it's an Ed25519 key
	privateKey, ok := privateKeyInterface.(ed25519.PrivateKey)
	if !ok {
		return "", fmt.Errorf("invalid key type: expected Ed25519 private key, got %T", privateKeyInterface)
	}

	// Extract seed (first 32 bytes of Ed25519 private key)
	seed := privateKey.Seed()

	// Extract public key (last 32 bytes of Ed25519 private key)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Encode to base64url (without padding)
	jwk := jwkKey{
		Kty: "OKP",
		Crv: "Ed25519",
		D:   base64.RawURLEncoding.EncodeToString(seed),
		X:   base64.RawURLEncoding.EncodeToString(publicKey),
	}

	// Marshal to JSON
	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWK: %w", err)
	}

	return string(jwkJSON), nil
}
