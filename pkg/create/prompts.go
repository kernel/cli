package create

import (
	"fmt"

	"github.com/kernel/cli/pkg/interactive"
)

// The prompts below are thin, per-field wrappers over pkg/interactive's
// prompt primitives. They perform no validation: ResolveInput owns
// validation and normalization for both the interactive and non-interactive
// flows, and the interactive flow re-resolves after each prompt.

// PromptName prompts for the app name. An empty entry falls back to
// DefaultAppName.
func PromptName(p interactive.Prompter) (string, error) {
	name, err := p.TextInput(
		"app name",
		"pass --name to set the app name (e.g. --name "+DefaultAppName+")",
		fmt.Sprintf("%s (%s)", AppNamePrompt, DefaultAppName),
	)
	if err != nil {
		return "", err
	}
	if name == "" {
		return DefaultAppName, nil
	}
	return name, nil
}

// PromptLanguage prompts for the application language.
func PromptLanguage(p interactive.Prompter) (string, error) {
	return p.Select(
		"language selection",
		"pass --language with one of: "+languageOptionsHint(),
		LanguagePrompt,
		SupportedLanguages,
	)
}

// PromptTemplate prompts for a template supported by the given (normalized)
// language.
func PromptTemplate(p interactive.Prompter, language string) (string, error) {
	templateKVs := GetSupportedTemplatesForLanguage(language)
	display, err := p.Select(
		"template selection",
		"pass --template with one of: "+templateOptionsHint(templateKVs),
		TemplatePrompt,
		templateKVs.GetTemplateDisplayValues(),
	)
	if err != nil {
		return "", err
	}
	return templateKVs.GetTemplateKeyFromValue(display)
}

// PromptOverwrite asks for confirmation before overwriting an existing
// directory.
func PromptOverwrite(p interactive.Prompter, dirName string) (bool, error) {
	return p.Confirm(
		fmt.Sprintf("overwrite existing directory '%s'", dirName),
		fmt.Sprintf("Directory %s already exists. Overwrite?", dirName),
	)
}
