package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var workloadIdentityNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

var workloadIdentityCmd = &cobra.Command{
	Use:     "workload-identity",
	Aliases: []string{"workload-identities"},
	Short:   "Manage workload identity federation",
	Long: "Manage trusted workload identity issuers, federation rules, and authentication events. " +
		"These commands require a signed-in organization member; Kernel API keys cannot manage workload identity resources.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(workloadIdentityCmd)
}

func validateWorkloadIdentityName(value, flagName string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("--%s is required", flagName)
	}
	if len(value) > 128 {
		return "", fmt.Errorf("--%s must be at most 128 characters", flagName)
	}
	if !workloadIdentityNamePattern.MatchString(value) {
		return "", fmt.Errorf("--%s must start with a letter or number and contain only letters, numbers, and hyphens", flagName)
	}
	return value, nil
}

func validateWorkloadIdentityDescription(value string) error {
	if len(value) > 4096 {
		return fmt.Errorf("--description must be at most 4096 characters")
	}
	return nil
}

func validateWorkloadIdentityListStatus(value string) (string, error) {
	switch value {
	case "", "active", "archived", "all":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --status %q: must be one of active, archived, all", value)
	}
}

func workloadIdentityResourceStatus(archived bool) string {
	if archived {
		return "archived"
	}
	return "active"
}
