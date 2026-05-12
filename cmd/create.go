package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kernel/cli/pkg/create"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// CreateCmd is a cobra-independent command handler for create operations
type CreateCmd struct{}

// Create executes the creating a new Kernel app logic
func (c CreateCmd) Create(ctx context.Context, ci create.CreateInput) error {
	appPath, err := filepath.Abs(ci.Name)
	if err != nil {
		return fmt.Errorf("failed to resolve app path: %w", err)
	}

	// Check if directory already exists and prompt for overwrite
	if _, err := os.Stat(appPath); err == nil {
		overwrite, err := create.PromptForOverwrite(ci.Name)
		if err != nil {
			return fmt.Errorf("failed to prompt for overwrite: %w", err)
		}

		if !overwrite {
			pterm.Warning.Println("Operation cancelled.")
			return nil
		}

		// Remove existing directory
		if err := os.RemoveAll(appPath); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}

	if err := os.MkdirAll(appPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	pterm.Printfln("\nCreating a new %s %s", ci.Language, ci.Template)

	spinner, _ := pterm.DefaultSpinner.Start("Copying template files...")

	if err := create.CopyTemplateFiles(appPath, ci.Language, ci.Template); err != nil {
		spinner.Fail("Failed to copy template files")
		return fmt.Errorf("failed to copy template files: %w", err)
	}
	spinner.Success()

	nextSteps, err := create.InstallDependencies(appPath, ci)
	if err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	pterm.Success.Println("🎉 Kernel app created successfully!")
	pterm.Println()
	pterm.FgYellow.Println(nextSteps)

	return nil
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application",
	Long:  buildCreateLongHelp(),
	Example: strings.Join([]string{
		"create --name my-app --language typescript --template anthropic-computer-use",
		"create -n my-app -l py -t sample-app",
	}, "\n"),
	RunE: runCreateApp,
}

func init() {
	createCmd.Flags().StringP("name", "n", "", "Name of the application")
	createCmd.Flags().StringP("language", "l", "", fmt.Sprintf("Language of the application (%s)", strings.Join(supportedLanguageDisplay(), ", ")))
	createCmd.Flags().StringP("template", "t", "", "Template to use for the application (see 'kernel create --help' for the full list)")
}

// supportedLanguageDisplay returns each supported language with its shorthand,
// e.g. ["typescript|ts", "python|py"], for inline flag-usage hints.
func supportedLanguageDisplay() []string {
	out := make([]string, 0, len(create.SupportedLanguages))
	for _, l := range create.SupportedLanguages {
		if s := create.LanguageShorthand(l); s != "" {
			out = append(out, l+"|"+s)
		} else {
			out = append(out, l)
		}
	}
	return out
}

// buildCreateLongHelp renders the Long help text for `kernel create`,
// listing supported languages and every template (with descriptions and
// which languages it supports) so agents and scripts can pick non-interactively.
func buildCreateLongHelp() string {
	var b strings.Builder
	b.WriteString("Commands for creating new Kernel applications.\n\n")
	b.WriteString("Pass --name, --language and --template to scaffold non-interactively;\n")
	b.WriteString("any omitted flag falls back to an interactive prompt.\n\n")

	b.WriteString("Languages:\n")
	for _, l := range create.SupportedLanguages {
		if s := create.LanguageShorthand(l); s != "" {
			fmt.Fprintf(&b, "  %s (shorthand: %s)\n", l, s)
		} else {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}

	keys := make([]string, 0, len(create.Templates))
	for k := range create.Templates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	keyWidth := 0
	for _, k := range keys {
		if len(k) > keyWidth {
			keyWidth = len(k)
		}
	}

	b.WriteString("\nTemplates:\n")
	for _, k := range keys {
		info := create.Templates[k]
		langs := append([]string(nil), info.Languages...)
		sort.Strings(langs)
		fmt.Fprintf(&b, "  %-*s  %s [%s]\n", keyWidth, k, info.Description, strings.Join(langs, ", "))
	}

	return strings.TrimRight(b.String(), "\n")
}

func runCreateApp(cmd *cobra.Command, args []string) error {
	appName, _ := cmd.Flags().GetString("name")
	language, _ := cmd.Flags().GetString("language")
	template, _ := cmd.Flags().GetString("template")

	appName, err := create.PromptForAppName(appName)
	if err != nil {
		return fmt.Errorf("failed to get app name: %w", err)
	}

	language, err = create.PromptForLanguage(language)
	if err != nil {
		return fmt.Errorf("failed to get language: %w", err)
	}

	template, err = create.PromptForTemplate(template, language)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	c := CreateCmd{}
	return c.Create(cmd.Context(), create.CreateInput{
		Name:     appName,
		Language: language,
		Template: template,
	})
}
