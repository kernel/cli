package cmd

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// AuthContextService defines the subset of the Kernel SDK auth context client that we use.
type AuthContextService interface {
	Get(ctx context.Context, opts ...option.RequestOption) (res *kernel.AuthContext, err error)
}

// AuthContextCmd handles auth context operations independent of cobra.
type AuthContextCmd struct {
	svc AuthContextService
}

type AuthContextGetInput struct {
	Output string
}

func (c AuthContextCmd) Get(ctx context.Context, in AuthContextGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	authCtx, err := c.svc.Get(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if authCtx == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(authCtx)
	}

	renderAuthContext(authCtx)
	return nil
}

func renderAuthContext(authCtx *kernel.AuthContext) {
	if authCtx == nil {
		pterm.Info.Println("No authentication context found")
		return
	}

	rows := pterm.TableData{
		{"Field", "Value"},
		{"Principal ID", authCtx.Principal.ID},
		{"Principal Type", authCtx.Principal.Type},
		{"Organization ID", authCtx.Organization.ID},
		{"Auth Method", authCtx.Authentication.Method},
		{"Auth Source", authCtx.Authentication.Source},
		// credential_id is null for session credentials.
		{"Credential ID", formatAuthContextOptional(authCtx.Authentication.CredentialID)},
		// A null project_id means the scope is organization-wide.
		{"Credential Scope", formatAuthContextScope(authCtx.Authorization.CredentialScope.ProjectID)},
		{"Effective Scope", formatAuthContextScope(authCtx.Authorization.EffectiveScope.ProjectID)},
	}
	PrintTableNoPad(rows, true)
}

func formatAuthContextOptional(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatAuthContextScope(projectID string) string {
	if projectID == "" {
		return "organization-wide"
	}
	return projectID
}

// --- Cobra wiring ---

var authContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Show the authentication context for the current credentials",
	Long: `Show the identity and authorization context resolved for requests made with the current credentials.

Displays the authenticated principal, organization, credential scope, and effective request scope.
Credential secrets are never returned.`,
	Args: cobra.NoArgs,
	RunE: runAuthContext,
}

func runAuthContext(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Auth.Context
	c := AuthContextCmd{svc: &svc}
	return c.Get(cmd.Context(), AuthContextGetInput{Output: output})
}

func init() {
	addJSONOutputFlag(authContextCmd)
	authCmd.AddCommand(authContextCmd)
}
