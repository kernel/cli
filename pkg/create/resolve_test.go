package create

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ResolveInput never prompts and never inspects the terminal; these tests
// exercise the problem model both flows consume.

func problemFields(problems []Problem) []Field {
	fields := make([]Field, len(problems))
	for i, p := range problems {
		fields[i] = p.Field
	}
	return fields
}

func TestResolveInputReportsAllProblems(t *testing.T) {
	t.Parallel()
	_, problems := ResolveInput(RawInput{})
	assert.Equal(t, []Field{FieldName, FieldLanguage, FieldTemplate}, problemFields(problems))

	msgs := ProblemMessages(problems)
	assert.Contains(t, msgs[0], "--name is required")
	assert.Contains(t, msgs[1], "--language is required")
	assert.Contains(t, msgs[2], "--template is required")
}

func TestResolveInputInvalidValues(t *testing.T) {
	t.Parallel()
	_, problems := ResolveInput(RawInput{Name: "bad name!", Language: "ruby", Template: "nope"})
	require.Len(t, problems, 3)
	assert.Contains(t, problems[0].Message, "--name 'bad name!' is invalid")
	assert.Contains(t, problems[1].Message, "--language 'ruby' is invalid")
	// Language is unknown, but the template exists for no language at all,
	// so it must still be reported in the same pass.
	assert.Contains(t, problems[2].Message, "--template 'nope' is invalid")
	// Provided-but-rejected values are marked Invalid so the interactive
	// flow warns before re-prompting; missing values are not.
	for _, p := range problems {
		assert.True(t, p.Invalid, "problem %q should be marked Invalid", p.Field)
	}
}

func TestResolveInputMissingValuesAreNotInvalid(t *testing.T) {
	t.Parallel()
	_, problems := ResolveInput(RawInput{})
	for _, p := range problems {
		assert.False(t, p.Invalid, "missing input %q should not be marked Invalid", p.Field)
	}
}

func TestResolveInputTemplateValidatedPerLanguage(t *testing.T) {
	t.Parallel()
	in, problems := ResolveInput(RawInput{Name: "my-app", Language: "ts", Template: "nope"})
	require.Len(t, problems, 1)
	assert.Equal(t, FieldTemplate, problems[0].Field)
	assert.Contains(t, problems[0].Message, "--template 'nope' is invalid for language 'typescript'")
	assert.Contains(t, problems[0].Message, TemplateSampleApp)
	// Resolved fields are returned even when problems remain, so the
	// interactive flow can prompt for the template of the chosen language.
	assert.Equal(t, LanguageTypeScript, in.Language)
	assert.Empty(t, in.Template)
}

func TestResolveInputExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-app"), 0o755))

	_, problems := ResolveInput(RawInput{Name: "my-app", Language: "ts", Template: TemplateSampleApp})
	require.Len(t, problems, 1)
	assert.Equal(t, FieldOverwrite, problems[0].Field)
	assert.Contains(t, problems[0].Message, "directory 'my-app' already exists")
	assert.Contains(t, problems[0].Message, "--yes")

	// --yes skips the overwrite confirmation, so it is not a problem.
	in, problems := ResolveInput(RawInput{Name: "my-app", Language: "ts", Template: TemplateSampleApp, SkipOverwriteConfirm: true})
	assert.Empty(t, problems)
	assert.True(t, in.SkipConfirm)
}

func TestResolveInputValid(t *testing.T) {
	t.Parallel()
	in, problems := ResolveInput(RawInput{Name: "my-app", Language: "py", Template: TemplateSampleApp})
	assert.Empty(t, problems)
	assert.Equal(t, "my-app", in.Name)
	assert.Equal(t, LanguagePython, in.Language)
	assert.Equal(t, TemplateSampleApp, in.Template)
	assert.False(t, in.SkipConfirm)
}

// Every Problem emitted by ResolveInput must carry its interactive
// resolution step: a problem the dispatcher cannot resolve would otherwise
// loop forever, which is exactly the failure class this package removes.
func TestEveryProblemCarriesResolutionStep(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-app"), 0o755))

	raws := []RawInput{
		{}, // all missing
		{Name: "bad name!", Language: "ruby", Template: "nope"},       // all invalid
		{Name: "my-app", Language: "ts", Template: TemplateSampleApp}, // overwrite
		{Name: "my-app", Language: "ts", Template: "nope"},            // per-language template
	}
	seen := map[Field]bool{}
	for _, raw := range raws {
		_, problems := ResolveInput(raw)
		for _, p := range problems {
			seen[p.Field] = true
			assert.NotNil(t, p.resolve, "problem %q must carry a resolution step", p.Field)
		}
	}
	// All fields exercised.
	assert.Equal(t, map[Field]bool{FieldName: true, FieldLanguage: true, FieldTemplate: true, FieldOverwrite: true}, seen)
}

// A Problem constructed without a resolution step (i.e. not by ResolveInput)
// must fail with an internal error naming the field rather than spin.
func TestProblemWithoutResolutionStepErrors(t *testing.T) {
	t.Parallel()
	var raw RawInput
	cancelled, err := Problem{Field: "future-field"}.Resolve(interactive.NewPrompterWithTerminal(true), &raw)
	require.Error(t, err)
	assert.False(t, cancelled)
	assert.Contains(t, err.Error(), "internal error")
	assert.Contains(t, err.Error(), "future-field")
}

// Resolution steps must fail fast through the injected Prompter in a
// non-interactive shell (they are also unreachable in practice because the
// dispatcher checks CanPrompt first).
func TestProblemResolutionFailsFastWhenNonInteractive(t *testing.T) {
	t.Parallel()
	raw := RawInput{}
	_, problems := ResolveInput(raw)
	require.NotEmpty(t, problems)
	_, err := problems[0].Resolve(interactive.NewPrompterWithTerminal(false), &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an interactive terminal")
}
