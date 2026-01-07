package create

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSupportedTemplatesForLanguage_Deterministic(t *testing.T) {
	// Run the function multiple times to ensure consistent ordering
	const iterations = 10
	language := LanguageTypeScript

	var firstResult TemplateKeyValues
	for i := 0; i < iterations; i++ {
		result := GetSupportedTemplatesForLanguage(language)

		if i == 0 {
			firstResult = result
		} else {
			// Verify that each iteration produces the same order
			assert.Equal(t, len(firstResult), len(result), "All iterations should return the same number of templates")
			for j := range result {
				assert.Equal(t, firstResult[j].Key, result[j].Key, "Template at index %d should be consistent across iterations", j)
				assert.Equal(t, firstResult[j].Value, result[j].Value, "Template value at index %d should be consistent across iterations", j)
			}
		}
	}
}

func TestTemplateKeyValues_GetTemplateDisplayValues(t *testing.T) {
	templates := TemplateKeyValues{
		{Key: "sample-app", Value: "Sample App - Implements basic Kernel apps"},
		{Key: "advanced-sample", Value: "Advanced Sample - Implements sample actions with advanced Kernel configs"},
	}

	displayValues := templates.GetTemplateDisplayValues()

	assert.Len(t, displayValues, 2)
	assert.Equal(t, "Sample App - Implements basic Kernel apps", displayValues[0])
	assert.Equal(t, "Advanced Sample - Implements sample actions with advanced Kernel configs", displayValues[1])
}

func TestTemplateKeyValues_GetTemplateKeyFromValue(t *testing.T) {
	templates := TemplateKeyValues{
		{Key: "sample-app", Value: "Sample App - Implements basic Kernel apps"},
		{Key: "advanced-sample", Value: "Advanced Sample - Implements sample actions with advanced Kernel configs"},
	}

	tests := []struct {
		name          string
		selectedValue string
		wantKey       string
		wantErr       bool
	}{
		{
			name:          "Valid value returns correct key",
			selectedValue: "Sample App - Implements basic Kernel apps",
			wantKey:       "sample-app",
			wantErr:       false,
		},
		{
			name:          "Invalid value returns error",
			selectedValue: "Non-existent template",
			wantKey:       "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := templates.GetTemplateKeyFromValue(tt.selectedValue)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantKey, key)
			}
		})
	}
}

func TestTemplateKeyValues_ContainsKey(t *testing.T) {
	templates := TemplateKeyValues{
		{Key: "sample-app", Value: "Sample App - Implements basic Kernel apps"},
		{Key: "advanced-sample", Value: "Advanced Sample - Implements sample actions with advanced Kernel configs"},
	}

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "Existing key returns true",
			key:  "sample-app",
			want: true,
		},
		{
			name: "Non-existing key returns false",
			key:  "non-existent",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := templates.ContainsKey(tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}
