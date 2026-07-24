package create

import (
	"fmt"
	"os"
	"slices"

	"github.com/kernel/cli/pkg/interactive"
)

// ValidateNonInteractive validates every scaffolding input without prompting
// and aggregates all missing or invalid flags into a single error, so a
// non-interactive caller (usually an AI agent) can fix everything in one
// retry. It also flags an existing target directory when skipOverwriteConfirm
// is false, since that would otherwise require an interactive confirmation.
func ValidateNonInteractive(name, language, template string, skipOverwriteConfirm bool) (CreateInput, error) {
	var problems []string

	// --name
	nameValid := false
	if name == "" {
		problems = append(problems, "--name is required (e.g. --name "+DefaultAppName+")")
	} else if err := validateAppName(name); err != nil {
		problems = append(problems, fmt.Sprintf("--name '%s' is invalid: %v", name, err))
	} else {
		nameValid = true
	}

	// --language
	lang := ""
	if language == "" {
		problems = append(problems, "--language is required: one of: "+languageOptionsHint())
	} else if l := NormalizeLanguage(language); slices.Contains(SupportedLanguages, l) {
		lang = l
	} else {
		problems = append(problems, fmt.Sprintf("--language '%s' is invalid: must be one of: %s", language, languageOptionsHint()))
	}

	// --template (valid values depend on --language)
	if lang != "" {
		templateKVs := GetSupportedTemplatesForLanguage(lang)
		if template == "" {
			problems = append(problems, "--template is required: one of: "+templateOptionsHint(templateKVs))
		} else if !templateKVs.ContainsKey(template) {
			problems = append(problems, fmt.Sprintf("--template '%s' is invalid for language '%s': must be one of: %s", template, lang, templateOptionsHint(templateKVs)))
		}
	} else if template == "" {
		problems = append(problems, "--template is required (run 'kernel create --help' for the full list)")
	} else if _, ok := Templates[template]; !ok {
		// Language is unknown, but the template doesn't exist for any
		// language, so report it now rather than on the next retry.
		problems = append(problems, fmt.Sprintf("--template '%s' is invalid (run 'kernel create --help' for the full list)", template))
	}

	// Overwriting the target directory would need a confirmation prompt.
	if nameValid && !skipOverwriteConfirm {
		if _, err := os.Stat(name); err == nil {
			problems = append(problems, fmt.Sprintf("directory '%s' already exists: pass --yes to overwrite it, or choose a different --name", name))
		}
	}

	if len(problems) > 0 {
		return CreateInput{}, interactive.ErrInputsRequired(problems)
	}

	return CreateInput{
		Name:        name,
		Language:    lang,
		Template:    template,
		SkipConfirm: skipOverwriteConfirm,
	}, nil
}
