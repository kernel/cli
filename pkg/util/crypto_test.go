package util

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePEMKey(t *testing.T) {
	tests := []struct {
		name    string
		pemData string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid Ed25519 PEM key",
			pemData: `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIJ+DYvh6SEqVTm50DFtMDoQikTmiCqirVv9mWG9qfSnF
-----END PRIVATE KEY-----`,
			wantErr: false,
		},
		{
			name:    "invalid PEM format",
			pemData: "not a pem key",
			wantErr: true,
			errMsg:  "failed to decode PEM block",
		},
		{
			name: "wrong PEM type",
			pemData: `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAJrQLj5P/89iXES9+vFgrIy29clF9CC/oPPsw3c5D0bs=
-----END PUBLIC KEY-----`,
			wantErr: true,
			errMsg:  "invalid PEM type",
		},
		{
			name: "invalid PKCS8 data",
			pemData: `-----BEGIN PRIVATE KEY-----
aW52YWxpZCBkYXRh
-----END PRIVATE KEY-----`,
			wantErr: true,
			errMsg:  "failed to parse PKCS#8 private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePEMKey(tt.pemData)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertJWKToPEM(t *testing.T) {
	tests := []struct {
		name       string
		jwkJSON    string
		wantErr    bool
		errMsg     string
		wantPubKey string // Expected base64url-encoded public key for validation
	}{
		{
			name: "valid JWK",
			jwkJSON: `{
				"kty": "OKP",
				"crv": "Ed25519",
				"d": "n4Ni-HpISpVObnQMW0wOhCKROaIKqKtW_2ZYb2p9KcU",
				"x": "JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs"
			}`,
			wantErr:    false,
			wantPubKey: "JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs",
		},
		{
			name:    "invalid JSON",
			jwkJSON: `{invalid json}`,
			wantErr: true,
			errMsg:  "failed to parse JWK",
		},
		{
			name: "wrong key type",
			jwkJSON: `{
				"kty": "RSA",
				"crv": "Ed25519",
				"d": "test"
			}`,
			wantErr: true,
			errMsg:  "invalid key type",
		},
		{
			name: "wrong curve",
			jwkJSON: `{
				"kty": "OKP",
				"crv": "Ed448",
				"d": "test"
			}`,
			wantErr: true,
			errMsg:  "invalid key type",
		},
		{
			name: "invalid base64url encoding",
			jwkJSON: `{
				"kty": "OKP",
				"crv": "Ed25519",
				"d": "not valid base64url!!!"
			}`,
			wantErr: true,
			errMsg:  "failed to decode private key",
		},
		{
			name: "invalid key size",
			jwkJSON: `{
				"kty": "OKP",
				"crv": "Ed25519",
				"d": "dGVzdA"
			}`,
			wantErr: true,
			errMsg:  "invalid private key size",
		},
		{
			name: "missing private key component",
			jwkJSON: `{
				"kty": "OKP",
				"crv": "Ed25519",
				"x": "JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs"
			}`,
			wantErr: true,
			errMsg:  "invalid private key size",
		},
		{
			name: "missing key type",
			jwkJSON: `{
				"crv": "Ed25519",
				"d": "n4Ni-HpISpVObnQMW0wOhCKROaIKqKtW_2ZYb2p9KcU"
			}`,
			wantErr: true,
			errMsg:  "invalid key type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pemData, err := ConvertJWKToPEM(tt.jwkJSON)
			if tt.wantErr {
				require.ErrorContains(t, err, tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, pemData)

			// Decode and validate the PEM structure
			block, rest := pem.Decode(pemData)
			require.NotNil(t, block, "Failed to decode PEM block")
			assert.Empty(t, rest, "Expected single PEM block, found extra data")
			assert.Equal(t, "PRIVATE KEY", block.Type)

			// Parse as PKCS#8 and verify it's Ed25519
			privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			require.NoError(t, err, "Failed to parse PKCS#8")

			ed25519Key, ok := privateKey.(ed25519.PrivateKey)
			require.True(t, ok, "Expected Ed25519 private key, got %T", privateKey)
			assert.Len(t, ed25519Key, ed25519.PrivateKeySize, "Invalid private key size")

			// Verify the public key matches expected value (if provided)
			if tt.wantPubKey != "" {
				pubKey := ed25519Key.Public().(ed25519.PublicKey)
				// Encode to base64url for comparison
				actualPubKey := base64.RawURLEncoding.EncodeToString(pubKey)
				assert.Equal(t, tt.wantPubKey, actualPubKey, "Public key mismatch")
			}

			// Roundtrip test: verify the key can sign and verify
			message := []byte("test message")
			signature := ed25519.Sign(ed25519Key, message)
			pubKey := ed25519Key.Public().(ed25519.PublicKey)
			assert.True(t, ed25519.Verify(pubKey, message, signature), "Signature verification failed")
		})
	}
}

func TestConvertPEMToJWK(t *testing.T) {
	tests := []struct {
		name       string
		pemData    string
		wantErr    bool
		errMsg     string
		wantKty    string
		wantCrv    string
		validateFn func(t *testing.T, jwkJSON string)
	}{
		{
			name: "valid Ed25519 PEM key",
			pemData: `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIJ+DYvh6SEqVTm50DFtMDoQikTmiCqirVv9mWG9qfSnF
-----END PRIVATE KEY-----`,
			wantErr: false,
			wantKty: "OKP",
			wantCrv: "Ed25519",
			validateFn: func(t *testing.T, jwkJSON string) {
				// Parse the JWK
				var jwk jwkKey
				err := json.Unmarshal([]byte(jwkJSON), &jwk)
				require.NoError(t, err)

				// Verify structure
				require.NotEmpty(t, jwk.D, "Private key component 'd' should not be empty")
				require.NotEmpty(t, jwk.X, "Public key component 'x' should not be empty")

				// Verify the private key can be decoded
				privKeyBytes, err := base64.RawURLEncoding.DecodeString(jwk.D)
				require.NoError(t, err)
				assert.Len(t, privKeyBytes, ed25519.SeedSize, "Private key should be 32 bytes")

				// Verify the public key can be decoded
				pubKeyBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
				require.NoError(t, err)
				assert.Len(t, pubKeyBytes, ed25519.PublicKeySize, "Public key should be 32 bytes")
			},
		},
		{
			name:    "invalid PEM format",
			pemData: "not a pem key",
			wantErr: true,
			errMsg:  "failed to decode PEM block",
		},
		{
			name: "wrong PEM type (public key)",
			pemData: `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAJrQLj5P/89iXES9+vFgrIy29clF9CC/oPPsw3c5D0bs=
-----END PUBLIC KEY-----`,
			wantErr: true,
			errMsg:  "invalid PEM type",
		},
		{
			name: "invalid PKCS8 data",
			pemData: `-----BEGIN PRIVATE KEY-----
aW52YWxpZCBkYXRh
-----END PRIVATE KEY-----`,
			wantErr: true,
			errMsg:  "failed to parse PKCS#8 private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwkJSON, err := ConvertPEMToJWK(tt.pemData)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, jwkJSON)

			// Parse and verify JWK structure
			var jwk jwkKey
			err = json.Unmarshal([]byte(jwkJSON), &jwk)
			require.NoError(t, err, "JWK should be valid JSON")

			// Verify basic fields
			assert.Equal(t, tt.wantKty, jwk.Kty, "Key type should be OKP")
			assert.Equal(t, tt.wantCrv, jwk.Crv, "Curve should be Ed25519")

			// Run custom validation if provided
			if tt.validateFn != nil {
				tt.validateFn(t, jwkJSON)
			}
		})
	}
}

func TestPEMToJWKRoundtrip(t *testing.T) {
	// Test that converting PEM -> JWK -> PEM produces equivalent keys
	originalPEM := `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIJ+DYvh6SEqVTm50DFtMDoQikTmiCqirVv9mWG9qfSnF
-----END PRIVATE KEY-----`

	// Convert PEM to JWK
	jwkJSON, err := ConvertPEMToJWK(originalPEM)
	require.NoError(t, err)
	require.NotEmpty(t, jwkJSON)

	// Convert JWK back to PEM
	pemData, err := ConvertJWKToPEM(jwkJSON)
	require.NoError(t, err)
	require.NotEmpty(t, pemData)

	// Parse both keys and verify they're the same
	block1, _ := pem.Decode([]byte(originalPEM))
	require.NotNil(t, block1)
	key1, err := x509.ParsePKCS8PrivateKey(block1.Bytes)
	require.NoError(t, err)
	ed25519Key1 := key1.(ed25519.PrivateKey)

	block2, _ := pem.Decode(pemData)
	require.NotNil(t, block2)
	key2, err := x509.ParsePKCS8PrivateKey(block2.Bytes)
	require.NoError(t, err)
	ed25519Key2 := key2.(ed25519.PrivateKey)

	// Compare private keys
	assert.Equal(t, ed25519Key1, ed25519Key2, "Keys should be identical after roundtrip")

	// Verify signatures match
	message := []byte("test message for signature verification")
	sig1 := ed25519.Sign(ed25519Key1, message)
	sig2 := ed25519.Sign(ed25519Key2, message)

	pubKey1 := ed25519Key1.Public().(ed25519.PublicKey)
	pubKey2 := ed25519Key2.Public().(ed25519.PublicKey)

	assert.True(t, ed25519.Verify(pubKey1, message, sig2), "Key1 should verify signature from Key2")
	assert.True(t, ed25519.Verify(pubKey2, message, sig1), "Key2 should verify signature from Key1")
}
