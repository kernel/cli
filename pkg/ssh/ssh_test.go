package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyURL(t *testing.T) {
	tests := []struct {
		name, cdpURL, want, wantErr string
	}{
		{
			name:   "metro root",
			cdpURL: "wss://proxy.metro.onkernel.com:8443/browser/cdp?jwt=a.b.c",
			want:   "wss://proxy.metro.onkernel.com:8443/browser/ssh?jwt=a.b.c",
		},
		{
			name:   "base path and extra query",
			cdpURL: "wss://proxy.metro.onkernel.com/metro/browser/cdp?jwt=a.b.c&other=1",
			want:   "wss://proxy.metro.onkernel.com/metro/browser/ssh?jwt=a.b.c",
		},
		{
			name:    "missing jwt",
			cdpURL:  "wss://proxy.metro.onkernel.com:8443/browser/cdp",
			wantErr: "no jwt",
		},
		{
			name:    "unexpected path",
			cdpURL:  "wss://proxy.metro.onkernel.com:8443/devtools/browser?jwt=a.b.c",
			wantErr: "unexpected CDP URL path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProxyURL(tt.cdpURL)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildSSHCommandUsesQuotedProxyURL(t *testing.T) {
	cmd := BuildSSHCommand("wss://proxy.metro.onkernel.com:8443/browser/ssh?jwt=a.b.c", "/tmp/key", Config{RemoteForward: "8080:localhost:3000"})
	assert.Equal(t, []string{
		"ssh",
		"-o", "ProxyCommand=websocat --binary 'wss://proxy.metro.onkernel.com:8443/browser/ssh?jwt=a.b.c'",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-i", "/tmp/key",
		"-R", "8080:localhost:3000",
		"root@localhost",
	}, cmd.Args)
}
