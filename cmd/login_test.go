package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/kernel/cli/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSpinner struct {
	stops int
}

func (s *testSpinner) Stop() error {
	s.stops++
	return nil
}

func TestCompleteLoginOutput(t *testing.T) {
	tests := []struct {
		name         string
		authErr      error
		saveErr      error
		cancel       bool
		wantErr      bool
		wantSave     bool
		wantSuccess  int
		wantErrors   int
		wantOutput   []string
		absentOutput []string
	}{
		{
			name:        "success",
			wantSave:    true,
			wantSuccess: 1,
			wantOutput: []string{
				"Successfully authenticated with Kernel!",
				"You can now use other Kernel CLI commands without setting KERNEL_API_KEY",
			},
			absentOutput: []string{"Authentication successful!", "ERROR"},
		},
		{
			name:       "consent denial",
			authErr:    auth.ErrAuthorizationDenied,
			wantErr:    true,
			wantErrors: 1,
			wantOutput: []string{"authorization denied; no new credentials were saved"},
			absentOutput: []string{
				"Authentication failed",
				"Successfully authenticated",
			},
		},
		{
			name:       "generic authentication failure",
			authErr:    errors.New("callback server failed"),
			wantErr:    true,
			wantErrors: 1,
			wantOutput: []string{"authentication failed: callback server failed"},
			absentOutput: []string{
				"Successfully authenticated",
			},
		},
		{
			name:       "credential save failure",
			saveErr:    errors.New("credential store unavailable"),
			wantErr:    true,
			wantSave:   true,
			wantErrors: 1,
			wantOutput: []string{
				"OAuth authorization completed, but credentials could not be saved: credential store unavailable",
				"fix credential storage and run 'kernel login' again",
			},
			absentOutput: []string{"SUCCESS", "Successfully authenticated"},
		},
		{
			name:       "cancellation",
			authErr:    context.Canceled,
			cancel:     true,
			wantErr:    true,
			wantErrors: 1,
			wantOutput: []string{"authentication cancelled by user"},
			absentOutput: []string{
				"authentication failed",
				"Successfully authenticated",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := capturePtermOutput(t)
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}

			spinner := &testSpinner{}
			saveCalled := false
			err := completeLogin(
				ctx,
				spinner,
				func(context.Context) (*auth.TokenStorage, error) {
					return &auth.TokenStorage{}, tt.authErr
				},
				func(*auth.TokenStorage) error {
					saveCalled = true
					return tt.saveErr
				},
			)

			if tt.wantErr {
				require.Error(t, err)
				renderCommandError(output, fang.Styles{
					ErrorText: lipgloss.NewStyle(),
					Program:   fang.Program{Flag: lipgloss.NewStyle()},
				}, err)
			} else {
				require.NoError(t, err)
			}

			rendered := ansiEscapes.ReplaceAllString(output.String(), "")
			assert.Equal(t, 1, spinner.stops)
			assert.Equal(t, tt.wantSave, saveCalled)
			assert.Equal(t, tt.wantSuccess, strings.Count(rendered, "SUCCESS"), rendered)
			assert.Equal(t, tt.wantErrors, strings.Count(rendered, "ERROR"), rendered)
			for _, want := range tt.wantOutput {
				assert.Contains(t, rendered, want)
			}
			for _, absent := range tt.absentOutput {
				assert.NotContains(t, rendered, absent)
			}
		})
	}
}
