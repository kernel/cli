package create

import (
	"fmt"
	"regexp"
	"slices"

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

// handleAppNamePrompt prompts the user for an app name interactively.
func handleAppNamePrompt() (string, error) {
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
func PromptForAppName(providedAppName string) (string, error) {
	// If no app name was provided, prompt the user
	if providedAppName == "" {
		return handleAppNamePrompt()
	}

	if err := validateAppName(providedAppName); err != nil {
		pterm.Warning.Printf("Invalid app name '%s': %v\n", providedAppName, err)
		pterm.Info.Println("Please provide a valid app name.")
		return handleAppNamePrompt()
	}

	return providedAppName, nil
}

func handleLanguagePrompt() (string, error) {
	l, err := pterm.DefaultInteractiveSelect.
		WithOptions(SupportedLanguages).
		WithDefaultText(LanguagePrompt).
		Show()
	if err != nil {
		return "", err
	}
	return l, nil
}

func PromptForLanguage(providedLanguage string) (string, error) {
	if providedLanguage == "" {
		return handleLanguagePrompt()
	}

	l := NormalizeLanguage(providedLanguage)
	if slices.Contains(SupportedLanguages, l) {
		return l, nil
	}

	pterm.Warning.Printfln("Language '%s' not found. Please select from available languages.\n", providedLanguage)
	return handleLanguagePrompt()
}

func handleTemplatePrompt(templateKVs TemplateKeyValues) (string, error) {
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

func PromptForTemplate(providedTemplate string, providedLanguage string) (string, error) {
	templateKVs := GetSupportedTemplatesForLanguage(NormalizeLanguage(providedLanguage))

	if providedTemplate == "" {
		return handleTemplatePrompt(templateKVs)
	}

	if templateKVs.ContainsKey(providedTemplate) {
		return providedTemplate, nil
	}

	pterm.Warning.Printfln("Template '%s' not found. Please select from available templates.\n", providedTemplate)
	return handleTemplatePrompt(templateKVs)
}

// PromptForOverwrite prompts the user to confirm overwriting an existing directory.
func PromptForOverwrite(dirName string) (bool, error) {
	overwrite, err := pterm.DefaultInteractiveConfirm.
		WithDefaultText(fmt.Sprintf("\nDirectory %s already exists. Overwrite?", dirName)).
		WithDefaultValue(false).
		Show()
	if err != nil {
		return false, fmt.Errorf("failed to prompt for overwrite: %w", err)
	}

	return overwrite, nil
}
