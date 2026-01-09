package util

import (
	"crypto/ed25519"
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

// JWKToPEM converts an Ed25519 JWK to PEM format (PKCS#8)
// If the input is already in PEM format, it validates and returns it as-is
func JWKToPEM(jwkJSON string) ([]byte, error) {
	// Check if input is already PEM-encoded
	if block, _ := pem.Decode([]byte(jwkJSON)); block != nil {
		if block.Type != "PRIVATE KEY" {
			return nil, fmt.Errorf("invalid PEM type: expected PRIVATE KEY, got %s", block.Type)
		}
		// TODO: Could add validation that it's actually an Ed25519 key
		return []byte(jwkJSON), nil
	}

	// Parse as JWK
	var key jwkKey
	if err := json.Unmarshal([]byte(jwkJSON), &key); err != nil {
		return nil, fmt.Errorf("failed to parse as JWK or PEM: %w", err)
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

	// Create PKCS#8 structure
	pkcs8Bytes, err := MarshalPKCS8PrivateKey(privateKey)
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

// MarshalPKCS8PrivateKey creates a PKCS#8 structure for Ed25519 private key
func MarshalPKCS8PrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	// PKCS#8 structure for Ed25519:
	// SEQUENCE {
	//   INTEGER 0 (version)
	//   SEQUENCE {
	//     OBJECT IDENTIFIER 1.3.101.112 (Ed25519)
	//   }
	//   OCTET STRING (containing the 32-byte seed as OCTET STRING)
	// }

	// Ed25519 OID: 1.3.101.112
	oid := []byte{0x06, 0x03, 0x2b, 0x65, 0x70}

	// Extract seed (first 32 bytes of private key)
	seed := key.Seed()

	// Inner OCTET STRING (seed) - RFC 8410: CurvePrivateKey
	innerOctetString := append([]byte{0x04, byte(len(seed))}, seed...)

	// Outer OCTET STRING wrapping the inner one - RFC 8410: privateKey field
	outerOctetString := append([]byte{0x04, byte(len(innerOctetString))}, innerOctetString...)

	// Algorithm identifier SEQUENCE
	algSeq := append([]byte{0x30, byte(len(oid))}, oid...)

	// Version (INTEGER 0)
	version := []byte{0x02, 0x01, 0x00}

	// Combine all parts
	content := append(version, algSeq...)
	content = append(content, outerOctetString...)

	// Outer SEQUENCE
	result := append([]byte{0x30, byte(len(content))}, content...)

	return result, nil
}
