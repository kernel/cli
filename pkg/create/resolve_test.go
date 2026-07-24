package create

import (
	"os"
	"path/filepath"
	"testing"

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
	_, problems := ResolveInput("", "", "", false)
	assert.Equal(t, []Field{FieldName, FieldLanguage, FieldTemplate}, problemFields(problems))

	msgs := ProblemMessages(problems)
	assert.Contains(t, msgs[0], "--name is required")
	assert.Contains(t, msgs[1], "--language is required")
	assert.Contains(t, msgs[2], "--template is required")
}

func TestResolveInputInvalidValues(t *testing.T) {
	_, problems := ResolveInput("bad name!", "ruby", "nope", false)
	require.Len(t, problems, 3)
	assert.Contains(t, problems[0].Message, "--name 'bad name!' is invalid")
	assert.Contains(t, problems[1].Message, "--language 'ruby' is invalid")
	// Language is unknown, but the template exists for no language at all,
	// so it must still be reported in the same pass.
	assert.Contains(t, problems[2].Message, "--template 'nope' is invalid")
}

func TestResolveInputTemplateValidatedPerLanguage(t *testing.T) {
	in, problems := ResolveInput("my-app", "ts", "nope", false)
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

	_, problems := ResolveInput("my-app", "ts", TemplateSampleApp, false)
	require.Len(t, problems, 1)
	assert.Equal(t, FieldOverwrite, problems[0].Field)
	assert.Contains(t, problems[0].Message, "directory 'my-app' already exists")
	assert.Contains(t, problems[0].Message, "--yes")

	// --yes skips the overwrite confirmation, so it is not a problem.
	in, problems := ResolveInput("my-app", "ts", TemplateSampleApp, true)
	assert.Empty(t, problems)
	assert.True(t, in.SkipConfirm)
}

func TestResolveInputValid(t *testing.T) {
	in, problems := ResolveInput("my-app", "py", TemplateSampleApp, false)
	assert.Empty(t, problems)
	assert.Equal(t, "my-app", in.Name)
	assert.Equal(t, LanguagePython, in.Language)
	assert.Equal(t, TemplateSampleApp, in.Template)
	assert.False(t, in.SkipConfirm)
}
