package create

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/pterm/pterm"
)

// validateAppName validates that an app name follows the required format.
// Returns an error if the name is invalid.
func validateAppName(val any) error {
	str, ok := val.(string)
	if !ok {
		return fmt.Errorf("invalid input type")
	}

	if len(str) == 0 {
		return fmt.Errorf("project name cannot be empty")
	}

	// Validate project name: only letters, numbers, underscores, and hyphens
	matched, err := regexp.MatchString(`^[A-Za-z\-_\d]+$`, str)
	if err != nil {
		return err
	}
	if !matched {
		return fmt.Errorf("project name may only include letters, numbers, underscores, and hyphens")
	}
	return nil
}

// languageOptionsHint renders the supported languages (with shorthands) for
// use in flag-usage hints, e.g. "typescript (ts), python (py)".
func languageOptionsHint() string {
	opts := make([]string, 0, len(SupportedLanguages))
	for _, l := range SupportedLanguages {
		if s := LanguageShorthand(l); s != "" {
			opts = append(opts, fmt.Sprintf("%s (%s)", l, s))
		} else {
			opts = append(opts, l)
		}
	}
	return strings.Join(opts, ", ")
}

// templateOptionsHint renders the template keys for use in flag-usage hints.
func templateOptionsHint(templateKVs TemplateKeyValues) string {
	keys := make([]string, 0, len(templateKVs))
	for _, kv := range templateKVs {
		keys = append(keys, kv.Key)
	}
	return strings.Join(keys, ", ")
}

// handleAppNamePrompt prompts the user for an app name interactively.
func handleAppNamePrompt() (string, error) {
	if !interactive.IsInteractive() {
		return "", interactive.ErrInputRequired("app name", "pass --name to set the app name (e.g. --name "+DefaultAppName+")")
	}

	promptText := fmt.Sprintf("%s (%s)", AppNamePrompt, DefaultAppName)
	appName, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText(promptText).
		Show()
	if err != nil {
		return "", err
	}

	if appName == "" {
		appName = DefaultAppName
	}

	if err := validateAppName(appName); err != nil {
		pterm.Warning.Printf("Invalid app name '%s': %v\n", appName, err)
		pterm.Info.Println("Please provide a valid app name.")
		return handleAppNamePrompt()
	}

	return appName, nil
}

// PromptForAppName validates the provided app name or prompts the user for one.
// If the provided name is invalid, it shows a warning and prompts the user.
// In a non-interactive shell it fails fast instead of prompting.
func PromptForAppName(providedAppName string) (string, error) {
	// If no app name was provided, prompt the user
	if providedAppName == "" {
		return handleAppNamePrompt()
	}

	if err := validateAppName(providedAppName); err != nil {
		if !interactive.IsInteractive() {
			return "", fmt.Errorf("invalid --name '%s': %w", providedAppName, err)
		}
		pterm.Warning.Printf("Invalid app name '%s': %v\n", providedAppName, err)
		pterm.Info.Println("Please provide a valid app name.")
		return handleAppNamePrompt()
	}

	return providedAppName, nil
}

func handleLanguagePrompt() (string, error) {
	if !interactive.IsInteractive() {
		return "", interactive.ErrInputRequired("language selection", "pass --language with one of: "+languageOptionsHint())
	}

	l, err := pterm.DefaultInteractiveSelect.
		WithOptions(SupportedLanguages).
		WithDefaultText(LanguagePrompt).
		Show()
	if err != nil {
		return "", err
	}
	return l, nil
}

// PromptForLanguage validates the provided language or prompts the user for
// one. In a non-interactive shell it fails fast instead of prompting.
func PromptForLanguage(providedLanguage string) (string, error) {
	if providedLanguage == "" {
		return handleLanguagePrompt()
	}

	l := NormalizeLanguage(providedLanguage)
	if slices.Contains(SupportedLanguages, l) {
		return l, nil
	}

	if !interactive.IsInteractive() {
		return "", fmt.Errorf("invalid --language '%s': must be one of: %s", providedLanguage, languageOptionsHint())
	}
	pterm.Warning.Printfln("Language '%s' not found. Please select from available languages.\n", providedLanguage)
	return handleLanguagePrompt()
}

func handleTemplatePrompt(templateKVs TemplateKeyValues) (string, error) {
	if !interactive.IsInteractive() {
		return "", interactive.ErrInputRequired("template selection", "pass --template with one of: "+templateOptionsHint(templateKVs))
	}

	template, err := pterm.DefaultInteractiveSelect.
		WithOptions(templateKVs.GetTemplateDisplayValues()).
		WithDefaultText(TemplatePrompt).
		WithMaxHeight(len(templateKVs)).
		Show()
	if err != nil {
		return "", err
	}

	return templateKVs.GetTemplateKeyFromValue(template)
}

// PromptForTemplate validates the provided template or prompts the user for
// one. In a non-interactive shell it fails fast instead of prompting.
func PromptForTemplate(providedTemplate string, providedLanguage string) (string, error) {
	templateKVs := GetSupportedTemplatesForLanguage(NormalizeLanguage(providedLanguage))

	if providedTemplate == "" {
		return handleTemplatePrompt(templateKVs)
	}

	if templateKVs.ContainsKey(providedTemplate) {
		return providedTemplate, nil
	}

	if !interactive.IsInteractive() {
		return "", fmt.Errorf("invalid --template '%s' for language '%s': must be one of: %s", providedTemplate, NormalizeLanguage(providedLanguage), templateOptionsHint(templateKVs))
	}
	pterm.Warning.Printfln("Template '%s' not found. Please select from available templates.\n", providedTemplate)
	return handleTemplatePrompt(templateKVs)
}

// PromptForOverwrite prompts the user to confirm overwriting an existing
// directory. In a non-interactive shell it fails fast instead of prompting;
// pass --yes to overwrite without confirmation.
func PromptForOverwrite(dirName string) (bool, error) {
	if !interactive.IsInteractive() {
		return false, interactive.ErrConfirmationRequired(fmt.Sprintf("overwrite existing directory '%s'", dirName))
	}

	overwrite, err := pterm.DefaultInteractiveConfirm.
		WithDefaultText(fmt.Sprintf("\nDirectory %s already exists. Overwrite?", dirName)).
		WithDefaultValue(false).
		Show()
	if err != nil {
		return false, fmt.Errorf("failed to prompt for overwrite: %w", err)
	}

	return overwrite, nil
}
