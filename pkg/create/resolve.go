package create

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
)

// Field identifies a scaffolding input that a Problem refers to.
type Field string

const (
	FieldName      Field = "name"
	FieldLanguage  Field = "language"
	FieldTemplate  Field = "template"
	FieldOverwrite Field = "overwrite"
)

// RawInput carries the raw `kernel create` flag values before resolution.
type RawInput struct {
	Name     string
	Language string
	Template string
	// SkipOverwriteConfirm is set by --yes, or after the user interactively
	// confirms overwriting an existing directory.
	SkipOverwriteConfirm bool
}

// Problem describes one input that is missing, invalid, or would require a
// confirmation. Message is phrased as the non-interactive fix instruction
// (naming the exact flag to pass); the interactive flow surfaces the same
// message as a warning before prompting for the field.
type Problem struct {
	Field   Field
	Message string
	// Invalid marks that a value was provided but rejected (as opposed to
	// missing); the interactive flow warns with Message before prompting.
	Invalid bool
	// resolve prompts for the field and applies the answer to raw.
	// ResolveInput sets it on every Problem it emits, so the
	// resolver/dispatcher contract is exhaustive by construction: a Problem
	// cannot exist without its interactive resolution step.
	resolve func(p interactive.Prompter, raw *RawInput) (cancelled bool, err error)
}

// Resolve prompts for this problem's field via p and applies the answer to
// raw. cancelled reports that the user declined a confirmation and the
// operation should stop.
func (pr Problem) Resolve(p interactive.Prompter, raw *RawInput) (cancelled bool, err error) {
	if pr.resolve == nil {
		// Defensive: only reachable for a Problem not built by ResolveInput.
		return false, fmt.Errorf("internal error: no interactive resolution step for input %q; pass the corresponding flag instead", pr.Field)
	}
	return pr.resolve(p, raw)
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

// Per-field interactive resolution steps. Each prompts for its field and
// applies the answer to raw; the caller re-resolves afterwards, so the
// template step can derive its options from the by-then-resolved language.
func resolveName(p interactive.Prompter, raw *RawInput) (bool, error) {
	v, err := PromptName(p)
	if err != nil {
		return false, err
	}
	raw.Name = v
	return false, nil
}

func resolveLanguage(p interactive.Prompter, raw *RawInput) (bool, error) {
	v, err := PromptLanguage(p)
	if err != nil {
		return false, err
	}
	raw.Language = v
	return false, nil
}

func resolveTemplate(p interactive.Prompter, raw *RawInput) (bool, error) {
	v, err := PromptTemplate(p, NormalizeLanguage(raw.Language))
	if err != nil {
		return false, err
	}
	raw.Template = v
	return false, nil
}

func resolveOverwrite(p interactive.Prompter, raw *RawInput) (bool, error) {
	ok, err := PromptOverwrite(p, raw.Name)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	raw.SkipOverwriteConfirm = true
	return false, nil
}

// ResolveInput is the single canonical resolver from raw flag values to a
// normalized CreateInput. It validates and normalizes every field, collecting
// a Problem for each one that is missing, invalid, or (for an existing target
// directory) would require an overwrite confirmation. It never prompts, but
// every Problem it emits carries the prompt step that resolves it.
//
// Both modes of `kernel create` consume its problem model: the interactive
// flow resolves one problem at a time and re-resolves, while the
// non-interactive flow reports all problems in a single fail-fast error.
// Problems are ordered name, language, template, overwrite so interactive
// resolution fixes the language before the language-dependent template list
// is needed.
//
// The returned CreateInput carries the fields that did resolve (with the
// language normalized) even when problems remain.
func ResolveInput(raw RawInput) (CreateInput, []Problem) {
	var problems []Problem

	// --name
	nameValid := false
	if raw.Name == "" {
		problems = append(problems, Problem{FieldName, "--name is required (e.g. --name " + DefaultAppName + ")", false, resolveName})
	} else if err := validateAppName(raw.Name); err != nil {
		problems = append(problems, Problem{FieldName, fmt.Sprintf("--name '%s' is invalid: %v", raw.Name, err), true, resolveName})
	} else {
		nameValid = true
	}

	// --language
	lang := ""
	if raw.Language == "" {
		problems = append(problems, Problem{FieldLanguage, "--language is required: one of: " + languageOptionsHint(), false, resolveLanguage})
	} else if l := NormalizeLanguage(raw.Language); slices.Contains(SupportedLanguages, l) {
		lang = l
	} else {
		problems = append(problems, Problem{FieldLanguage, fmt.Sprintf("--language '%s' is invalid: must be one of: %s", raw.Language, languageOptionsHint()), true, resolveLanguage})
	}

	// --template (valid values depend on --language)
	templateValid := false
	if lang != "" {
		templateKVs := GetSupportedTemplatesForLanguage(lang)
		if raw.Template == "" {
			problems = append(problems, Problem{FieldTemplate, "--template is required: one of: " + templateOptionsHint(templateKVs), false, resolveTemplate})
		} else if templateKVs.ContainsKey(raw.Template) {
			templateValid = true
		} else {
			problems = append(problems, Problem{FieldTemplate, fmt.Sprintf("--template '%s' is invalid for language '%s': must be one of: %s", raw.Template, lang, templateOptionsHint(templateKVs)), true, resolveTemplate})
		}
	} else if raw.Template == "" {
		problems = append(problems, Problem{FieldTemplate, "--template is required (run 'kernel create --help' for the full list)", false, resolveTemplate})
	} else if _, ok := Templates[raw.Template]; !ok {
		// Language is unknown, but the template doesn't exist for any
		// language, so report it now rather than on the next retry.
		problems = append(problems, Problem{FieldTemplate, fmt.Sprintf("--template '%s' is invalid (run 'kernel create --help' for the full list)", raw.Template), true, resolveTemplate})
	}

	// Overwriting the target directory would need a confirmation prompt.
	if nameValid && !raw.SkipOverwriteConfirm {
		if _, err := os.Stat(raw.Name); err == nil {
			problems = append(problems, Problem{FieldOverwrite, fmt.Sprintf("directory '%s' already exists: pass --yes to overwrite it, or choose a different --name", raw.Name), false, resolveOverwrite})
		}
	}

	in := CreateInput{
		Name:        raw.Name,
		Language:    lang,
		SkipConfirm: raw.SkipOverwriteConfirm,
	}
	if templateValid {
		in.Template = raw.Template
	}
	return in, problems
}
