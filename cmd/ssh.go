package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kernel/cli/pkg/ssh"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <id>",
	Short: "Open an interactive SSH session to a browser VM",
	Long: `Establish an SSH connection to a running browser VM.

By default, generates an ephemeral SSH keypair and opens an interactive shell.
Use -i to specify an existing SSH private key instead.

Port forwarding uses standard SSH syntax:
  -L localport:host:remoteport   Forward local port to remote
  -R remoteport:host:localport   Forward remote port to local

Examples:
  # Interactive shell
  kernel browsers ssh abc123def456

  # Expose local dev server (port 3000) on VM port 8080
  kernel browsers ssh abc123def456 -R 8080:localhost:3000

  # Access VM's port 5432 locally
  kernel browsers ssh abc123def456 -L 5432:localhost:5432

  # Use existing SSH key
  kernel browsers ssh abc123def456 -i ~/.ssh/id_ed25519`,
	Args: cobra.ExactArgs(1),
	RunE: runSSH,
}

func init() {
	sshCmd.Flags().StringP("identity", "i", "", "Path to SSH private key (generates ephemeral if not provided)")
	sshCmd.Flags().StringP("local-forward", "L", "", "Local port forwarding (localport:host:remoteport)")
	sshCmd.Flags().StringP("remote-forward", "R", "", "Remote port forwarding (remoteport:host:localport)")
	sshCmd.Flags().Bool("setup-only", false, "Setup SSH on VM without connecting")
}

func runSSH(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client := getKernelClient(cmd)
	browserID := args[0]

	identityFile, _ := cmd.Flags().GetString("identity")
	localForward, _ := cmd.Flags().GetString("local-forward")
	remoteForward, _ := cmd.Flags().GetString("remote-forward")
	setupOnly, _ := cmd.Flags().GetBool("setup-only")

	cfg := ssh.Config{
		BrowserID:     browserID,
		IdentityFile:  identityFile,
		LocalForward:  localForward,
		RemoteForward: remoteForward,
		SetupOnly:     setupOnly,
	}

	return connectSSH(ctx, client, cfg)
}

func connectSSH(ctx context.Context, client kernel.Client, cfg ssh.Config) error {
	// Check websocat is installed locally
	if err := ssh.CheckWebsocatInstalled(); err != nil {
		return err
	}

	// Get browser info
	pterm.Info.Printf("Getting browser %s info...\n", cfg.BrowserID)
	browser, err := client.Browsers.Get(ctx, cfg.BrowserID, kernel.BrowserGetParams{})
	if err != nil {
		return fmt.Errorf("failed to get browser: %w", err)
	}

	// Extract VM domain from live view URL or CDP URL
	var vmDomain string
	if browser.BrowserLiveViewURL != "" {
		vmDomain, err = ssh.ExtractVMDomain(browser.BrowserLiveViewURL)
	} else if browser.CdpWsURL != "" {
		vmDomain, err = ssh.ExtractVMDomain(browser.CdpWsURL)
	} else {
		return fmt.Errorf("browser has no live view URL or CDP URL - cannot determine VM domain")
	}
	if err != nil {
		return fmt.Errorf("failed to extract VM domain: %w", err)
	}
	pterm.Info.Printf("VM domain: %s\n", vmDomain)

	// Generate or load SSH keypair
	var privateKeyPEM, publicKey string
	var keyFile string
	var cleanupKey bool

	if cfg.IdentityFile != "" {
		// Use provided key
		pterm.Info.Printf("Using SSH key: %s\n", cfg.IdentityFile)
		keyFile = cfg.IdentityFile

		// Read public key to inject into VM
		// Try to read the .pub file
		pubKeyPath := cfg.IdentityFile + ".pub"
		pubKeyData, err := os.ReadFile(pubKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read public key %s: %w (ensure .pub file exists alongside private key)", pubKeyPath, err)
		}
		publicKey = strings.TrimSpace(string(pubKeyData))
	} else {
		// Generate ephemeral keypair
		pterm.Info.Println("Generating ephemeral SSH keypair...")
		keyPair, err := ssh.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate SSH keypair: %w", err)
		}
		privateKeyPEM = keyPair.PrivateKeyPEM
		publicKey = keyPair.PublicKeyOpenSSH

		// Write to temp file
		keyFile, err = ssh.WriteTempKey(privateKeyPEM, browser.SessionID)
		if err != nil {
			return fmt.Errorf("failed to write temp key: %w", err)
		}
		cleanupKey = true
		pterm.Debug.Printf("Temp key file: %s\n", keyFile)
	}

	// Cleanup temp key on exit
	if cleanupKey {
		defer func() {
			pterm.Debug.Printf("Cleaning up temp key: %s\n", keyFile)
			os.Remove(keyFile)
		}()
	}

	// Setup SSH services on VM
	pterm.Info.Println("Setting up SSH services on VM...")
	if err := setupVMSSH(ctx, client, browser.SessionID, publicKey); err != nil {
		return fmt.Errorf("failed to setup SSH on VM: %w", err)
	}
	pterm.Success.Println("SSH services running on VM")

	if cfg.SetupOnly {
		pterm.Info.Println("\n--setup-only specified, not connecting.")
		pterm.Info.Printf("To connect manually:\n")
		pterm.Info.Printf("  ssh -o 'ProxyCommand=websocat --binary wss://%s:2222' -i %s root@localhost\n", vmDomain, keyFile)
		return nil
	}

	// Build and run SSH command
	pterm.Info.Println("Connecting via SSH...")
	sshCmd := ssh.BuildSSHCommand(vmDomain, keyFile, cfg)

	// Connect stdin/stdout/stderr
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	// Handle signals to pass to SSH process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			if sshCmd.Process != nil {
				sshCmd.Process.Signal(sig)
			}
		}
	}()
	defer signal.Stop(sigCh)

	// Run SSH (blocks until session ends)
	if err := sshCmd.Run(); err != nil {
		// Exit code 255 is common for SSH errors, provide more context
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 255 {
				return fmt.Errorf("SSH connection failed (exit 255). Check that:\n  1. websocat is installed and working\n  2. The browser VM is still running\n  3. Port 2222 is accessible on the VM")
			}
		}
		return fmt.Errorf("SSH session ended with error: %w", err)
	}

	return nil
}

// setupVMSSH installs and configures sshd + websocat on the VM using process.exec
func setupVMSSH(ctx context.Context, client kernel.Client, sessionID, publicKey string) error {
	// First check if services are already running
	checkScript := ssh.CheckServicesScript()
	checkResp, err := client.Browsers.Process.Exec(ctx, sessionID, kernel.BrowserProcessExecParams{
		Command: "/bin/bash",
		Args:    []string{"-c", checkScript},
		AsRoot:  kernel.Opt(true),
	})
	if err != nil {
		pterm.Debug.Printf("Check services failed (will run setup): %v\n", err)
	} else if checkResp != nil && checkResp.StdoutB64 != "" {
		stdout, _ := base64.StdEncoding.DecodeString(checkResp.StdoutB64)
		if strings.TrimSpace(string(stdout)) == "RUNNING" {
			pterm.Info.Println("SSH services already running, injecting key...")
			// Just inject the key
			return injectSSHKey(ctx, client, sessionID, publicKey)
		}
	}

	// Run full setup script
	setupScript := ssh.SetupScript(publicKey)
	resp, err := client.Browsers.Process.Exec(ctx, sessionID, kernel.BrowserProcessExecParams{
		Command:    "/bin/bash",
		Args:       []string{"-c", setupScript},
		AsRoot:     kernel.Opt(true),
		TimeoutSec: kernel.Opt(int64(120)), // Allow 2 minutes for package install
	})
	if err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	if resp.ExitCode != 0 {
		// Decode and show stderr for debugging
		var stderr string
		if resp.StderrB64 != "" {
			stderrBytes, _ := base64.StdEncoding.DecodeString(resp.StderrB64)
			stderr = string(stderrBytes)
		}
		var stdout string
		if resp.StdoutB64 != "" {
			stdoutBytes, _ := base64.StdEncoding.DecodeString(resp.StdoutB64)
			stdout = string(stdoutBytes)
		}
		return fmt.Errorf("setup script failed (exit %d):\nstdout: %s\nstderr: %s", resp.ExitCode, stdout, stderr)
	}

	// Log setup output for debugging
	if resp.StdoutB64 != "" {
		stdout, _ := base64.StdEncoding.DecodeString(resp.StdoutB64)
		pterm.Debug.Printf("Setup output:\n%s\n", string(stdout))
	}

	return nil
}

// injectSSHKey adds a public key to authorized_keys (when services already running)
func injectSSHKey(ctx context.Context, client kernel.Client, sessionID, publicKey string) error {
	escapedKey := strings.ReplaceAll(publicKey, "'", "'\"'\"'")
	script := fmt.Sprintf(`mkdir -p /root/.ssh && chmod 700 /root/.ssh && echo '%s' >> /root/.ssh/authorized_keys && chmod 600 /root/.ssh/authorized_keys`, escapedKey)

	resp, err := client.Browsers.Process.Exec(ctx, sessionID, kernel.BrowserProcessExecParams{
		Command: "/bin/bash",
		Args:    []string{"-c", script},
		AsRoot:  kernel.Opt(true),
	})
	if err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	if resp.ExitCode != 0 {
		return fmt.Errorf("key injection failed (exit %d)", resp.ExitCode)
	}

	return nil
}
