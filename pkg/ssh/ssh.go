// Package ssh provides SSH connectivity to Kernel browser VMs.
package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Config holds SSH connection configuration
type Config struct {
	BrowserID     string
	IdentityFile  string // empty = generate ephemeral
	LocalForward  string // -L flag value
	RemoteForward string // -R flag value
	SetupOnly     bool
	Output        string // "json" for machine-readable output
}

// KeyPair holds an SSH keypair
type KeyPair struct {
	PrivateKeyPEM    string // PEM-encoded private key (OpenSSH format)
	PublicKeyOpenSSH string // OpenSSH authorized_keys format
}

// GenerateKeyPair creates an ed25519 SSH keypair suitable for OpenSSH.
// Returns the private key in PEM format and public key in authorized_keys format.
func GenerateKeyPair() (*KeyPair, error) {
	// Generate ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}

	// Format public key for authorized_keys
	publicKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))

	// Marshal private key to OpenSSH PEM format
	pemBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privateKeyPEM := string(pem.EncodeToMemory(pemBlock))

	return &KeyPair{
		PrivateKeyPEM:    privateKeyPEM,
		PublicKeyOpenSSH: publicKeyStr,
	}, nil
}

// ExtractVMDomain extracts the VM FQDN from a CDP WebSocket URL by decoding the JWT.
// The CDP URL contains a JWT with the actual Unikraft FQDN in the payload.
// Example CDP URL: wss://proxy.xxx.dev.onkernel.com:8443/browser/cdp?jwt=eyJ...
// The JWT payload contains: {"session": {"fqdn": "actual-vm-domain.onkernel.app"}}
func ExtractVMDomain(cdpURL string) (string, error) {
	if cdpURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	parsed, err := url.Parse(cdpURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Extract JWT from query parameter
	jwt := parsed.Query().Get("jwt")
	if jwt == "" {
		// Fallback to hostname if no JWT (shouldn't happen in practice)
		host := parsed.Hostname()
		if host == "" {
			return "", fmt.Errorf("no hostname in URL: %s", cdpURL)
		}
		return host, nil
	}

	// JWT is header.payload.signature - we need the payload (middle part)
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT format")
	}

	// Decode base64url payload
	payload := parts[1]
	// Add padding if needed (base64url may omit padding)
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	// Convert base64url to standard base64
	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// Parse JSON payload
	var claims struct {
		Session struct {
			FQDN string `json:"fqdn"`
		} `json:"session"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", fmt.Errorf("failed to parse JWT payload: %w", err)
	}

	if claims.Session.FQDN == "" {
		return "", fmt.Errorf("no FQDN in JWT payload")
	}

	return claims.Session.FQDN, nil
}

// CheckWebsocatInstalled verifies websocat is available in PATH.
// Returns nil if found, error with install instructions if not.
func CheckWebsocatInstalled() error {
	_, err := exec.LookPath("websocat")
	if err != nil {
		return fmt.Errorf(`websocat is required but not found in PATH

Install websocat:
  macOS:   brew install websocat
  Linux:   curl -fsSL https://github.com/vi/websocat/releases/download/v1.13.0/websocat.x86_64-unknown-linux-musl -o /usr/local/bin/websocat && chmod +x /usr/local/bin/websocat
  Windows: Download from https://github.com/vi/websocat/releases`)
	}
	return nil
}

// WriteTempKey writes the private key to a temporary file and returns the path.
// The caller is responsible for cleaning up the file.
func WriteTempKey(privateKeyPEM string, sessionID string) (string, error) {
	// Create temp file with restricted permissions
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kernel-ssh-%s-*", sessionID))
	if err != nil {
		return "", fmt.Errorf("failed to create temp key file: %w", err)
	}

	// Set permissions before writing (SSH requires 600)
	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to set key file permissions: %w", err)
	}

	if _, err := tmpFile.WriteString(privateKeyPEM); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write key file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to close key file: %w", err)
	}

	return tmpFile.Name(), nil
}

// BuildSSHCommand constructs the SSH command with websocat ProxyCommand.
func BuildSSHCommand(vmDomain, keyFile string, cfg Config) *exec.Cmd {
	// Build websocat ProxyCommand - connect to port 2222 for SSH websocket bridge
	proxyCmd := fmt.Sprintf("websocat --binary wss://%s:2222", vmDomain)

	args := []string{
		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR", // Suppress warnings about host key
		"-i", keyFile,
	}

	// Add port forwarding if specified
	if cfg.LocalForward != "" {
		args = append(args, "-L", cfg.LocalForward)
	}
	if cfg.RemoteForward != "" {
		args = append(args, "-R", cfg.RemoteForward)
	}

	// Connect as root - the actual hostname doesn't matter since ProxyCommand handles it
	args = append(args, "root@localhost")

	return exec.Command("ssh", args...)
}
