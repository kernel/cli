package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kernel/cli/pkg/create"
	"github.com/kernel/cli/pkg/interactive"
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

	// Check if directory already exists and prompt for overwrite. This is a
	// backstop for direct callers; runCreateApp resolves the overwrite
	// confirmation before calling Create.
	if _, err := os.Stat(appPath); err == nil {
		if !ci.SkipConfirm {
			overwrite, err := create.PromptOverwrite(ci.Name)
			if err != nil {
				return err
			}

			if !overwrite {
				pterm.Warning.Println("Operation cancelled.")
				return nil
			}
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
	createCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts (overwrite an existing directory without asking)")
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
	b.WriteString("any omitted flag falls back to an interactive prompt. In a\n")
	b.WriteString("non-interactive shell the command fails fast instead of prompting.\n")
	b.WriteString("Pass --yes to overwrite an existing directory without confirmation.\n\n")

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
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	c := CreateCmd{}

	// create.ResolveInput is the single resolver from raw flags to a
	// normalized CreateInput for both modes. Interactively, each remaining
	// problem is resolved by prompting for that field and re-resolving (so
	// e.g. the template list reflects the chosen language). Non-interactively,
	// every problem is reported at once in a single fail-fast error.
	for {
		in, problems := create.ResolveInput(appName, language, template, skipConfirm)
		if len(problems) == 0 {
			return c.Create(cmd.Context(), in)
		}
		if !interactive.IsInteractive() {
			return interactive.ErrInputsRequired(create.ProblemMessages(problems))
		}

		p := problems[0]
		switch p.Field {
		case create.FieldName:
			if appName != "" {
				pterm.Warning.Println(p.Message)
			}
			v, err := create.PromptName()
			if err != nil {
				return err
			}
			appName = v
		case create.FieldLanguage:
			if language != "" {
				pterm.Warning.Println(p.Message)
			}
			v, err := create.PromptLanguage()
			if err != nil {
				return err
			}
			language = v
		case create.FieldTemplate:
			if template != "" {
				pterm.Warning.Println(p.Message)
			}
			v, err := create.PromptTemplate(in.Language)
			if err != nil {
				return err
			}
			template = v
		case create.FieldOverwrite:
			ok, err := create.PromptOverwrite(appName)
			if err != nil {
				return err
			}
			if !ok {
				pterm.Warning.Println("Operation cancelled.")
				return nil
			}
			skipConfirm = true
		}
	}
}
