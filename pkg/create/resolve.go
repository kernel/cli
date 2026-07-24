package create

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// Field identifies a scaffolding input that a Problem refers to.
type Field string

const (
	FieldName      Field = "name"
	FieldLanguage  Field = "language"
	FieldTemplate  Field = "template"
	FieldOverwrite Field = "overwrite"
)

// Problem describes one input that is missing, invalid, or would require a
// confirmation. Message is phrased as the non-interactive fix instruction
// (naming the exact flag to pass); the interactive flow surfaces the same
// message as a warning before prompting for the field.
type Problem struct {
	Field   Field
	Message string
}

// ProblemMessages extracts the messages from problems, in order.
func ProblemMessages(problems []Problem) []string {
	msgs := make([]string, len(problems))
	for i, p := range problems {
		msgs[i] = p.Message
	}
	return msgs
}

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

// ResolveInput is the single canonical resolver from raw flag values to a
// normalized CreateInput. It validates and normalizes every field, collecting
// a Problem for each one that is missing, invalid, or (for an existing target
// directory) would require an overwrite confirmation. It never prompts.
//
// Both modes of `kernel create` consume its problem model: the interactive
// flow prompts to resolve one problem at a time and re-resolves, while the
// non-interactive flow reports all problems in a single fail-fast error.
// Problems are ordered name, language, template, overwrite so interactive
// resolution fixes the language before the language-dependent template list
// is needed.
//
// The returned CreateInput carries the fields that did resolve (with the
// language normalized) even when problems remain.
func ResolveInput(name, language, template string, skipOverwriteConfirm bool) (CreateInput, []Problem) {
	var problems []Problem

	// --name
	nameValid := false
	if name == "" {
		problems = append(problems, Problem{FieldName, "--name is required (e.g. --name " + DefaultAppName + ")"})
	} else if err := validateAppName(name); err != nil {
		problems = append(problems, Problem{FieldName, fmt.Sprintf("--name '%s' is invalid: %v", name, err)})
	} else {
		nameValid = true
	}

	// --language
	lang := ""
	if language == "" {
		problems = append(problems, Problem{FieldLanguage, "--language is required: one of: " + languageOptionsHint()})
	} else if l := NormalizeLanguage(language); slices.Contains(SupportedLanguages, l) {
		lang = l
	} else {
		problems = append(problems, Problem{FieldLanguage, fmt.Sprintf("--language '%s' is invalid: must be one of: %s", language, languageOptionsHint())})
	}

	// --template (valid values depend on --language)
	templateValid := false
	if lang != "" {
		templateKVs := GetSupportedTemplatesForLanguage(lang)
		if template == "" {
			problems = append(problems, Problem{FieldTemplate, "--template is required: one of: " + templateOptionsHint(templateKVs)})
		} else if templateKVs.ContainsKey(template) {
			templateValid = true
		} else {
			problems = append(problems, Problem{FieldTemplate, fmt.Sprintf("--template '%s' is invalid for language '%s': must be one of: %s", template, lang, templateOptionsHint(templateKVs))})
		}
	} else if template == "" {
		problems = append(problems, Problem{FieldTemplate, "--template is required (run 'kernel create --help' for the full list)"})
	} else if _, ok := Templates[template]; !ok {
		// Language is unknown, but the template doesn't exist for any
		// language, so report it now rather than on the next retry.
		problems = append(problems, Problem{FieldTemplate, fmt.Sprintf("--template '%s' is invalid (run 'kernel create --help' for the full list)", template)})
	}

	// Overwriting the target directory would need a confirmation prompt.
	if nameValid && !skipOverwriteConfirm {
		if _, err := os.Stat(name); err == nil {
			problems = append(problems, Problem{FieldOverwrite, fmt.Sprintf("directory '%s' already exists: pass --yes to overwrite it, or choose a different --name", name)})
		}
	}

	in := CreateInput{
		Name:        name,
		Language:    lang,
		SkipConfirm: skipOverwriteConfirm,
	}
	if templateValid {
		in.Template = template
	}
	return in, problems
}
