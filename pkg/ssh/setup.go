package ssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/kernel-go-sdk"
)

// BrowserProcessService defines the interface for executing commands on the VM.
// This matches the kernel SDK's BrowserProcessService.
type BrowserProcessService interface {
	Exec(ctx context.Context, id string, body kernel.BrowserProcessExecParams, opts ...interface{}) (*kernel.BrowserProcessExecResponse, error)
}

// SetupScript generates the bash script to setup SSH services on the VM.
func SetupScript(publicKey string) string {
	// Escape the public key for safe embedding in shell script
	escapedKey := strings.ReplaceAll(publicKey, "'", "'\"'\"'")

	return fmt.Sprintf(`#!/bin/bash
set -e

echo "=== Setting up SSH services on VM ==="

# Install openssh-server if needed
if ! command -v sshd &>/dev/null; then
    echo "Installing openssh-server..."
    apt-get update -qq
    apt-get install -y --no-install-recommends openssh-server
fi

# Create sshd privilege separation directory (required for sshd to run)
echo "Creating sshd directories..."
mkdir -p /run/sshd
chmod 755 /run/sshd

# Configure sshd
echo "Configuring sshd..."
mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/kernel.conf << 'SSHD_EOF'
GatewayPorts yes
TCPKeepAlive yes
PermitRootLogin prohibit-password
PasswordAuthentication no
PubkeyAuthentication yes
SSHD_EOF

# Install websocat if needed
if ! command -v websocat &>/dev/null; then
    echo "Installing websocat..."
    curl -fsSL https://github.com/vi/websocat/releases/download/v1.13.0/websocat.x86_64-unknown-linux-musl \
        -o /usr/local/bin/websocat && chmod +x /usr/local/bin/websocat
fi

# Create supervisor log directory
mkdir -p /var/log/supervisord

# Create sshd supervisor config
echo "Creating supervisor configs..."
cat > /etc/supervisor/conf.d/services/sshd.conf << 'SUPER_EOF'
[program:sshd]
command=/usr/sbin/sshd -D -e
autostart=false
autorestart=true
startsecs=2
stdout_logfile=/var/log/supervisord/sshd
redirect_stderr=true
SUPER_EOF

# Create websocat supervisor config
cat > /etc/supervisor/conf.d/services/websocat-ssh.conf << 'SUPER_EOF'
[program:websocat-ssh]
command=/usr/local/bin/websocat --binary ws-l:0.0.0.0:2222 tcp:127.0.0.1:22
autostart=false
autorestart=true
startsecs=2
stdout_logfile=/var/log/supervisord/websocat-ssh
redirect_stderr=true
SUPER_EOF

# Inject SSH public key
echo "Injecting SSH public key..."
mkdir -p /root/.ssh && chmod 700 /root/.ssh
echo '%s' >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys

# Generate host keys if they don't exist
if [ ! -f /etc/ssh/ssh_host_ed25519_key ]; then
    echo "Generating SSH host keys..."
    ssh-keygen -A
fi

# Start services via supervisor
echo "Starting SSH services..."
supervisorctl reread
supervisorctl update
supervisorctl start sshd websocat-ssh

echo "=== SSH setup complete ==="
`, escapedKey)
}

// CheckServicesScript returns a script to check if SSH services are already running.
func CheckServicesScript() string {
	return `#!/bin/bash
# Check if both sshd and websocat-ssh are running
sshd_status=$(supervisorctl status sshd 2>/dev/null | grep -c RUNNING || echo 0)
websocat_status=$(supervisorctl status websocat-ssh 2>/dev/null | grep -c RUNNING || echo 0)

if [ "$sshd_status" = "1" ] && [ "$websocat_status" = "1" ]; then
    echo "RUNNING"
else
    echo "NOT_RUNNING"
fi
`
}
