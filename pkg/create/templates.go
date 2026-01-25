package create

import (
	"fmt"
	"slices"
	"sort"
)

// Template key constants
const (
	TemplateSampleApp                  = "sample-app"
	TemplateCaptchaSolver              = "captcha-solver"
	TemplateAnthropicComputerUse       = "anthropic-computer-use"
	TemplateOpenAIComputerUse          = "openai-computer-use"
	TemplateMagnitude                  = "magnitude"
	TemplateGeminiComputerUse          = "gemini-computer-use"
	TemplateBrowserUse                 = "browser-use"
	TemplateStagehand                  = "stagehand"
	TemplateOpenAGIComputerUse         = "openagi-computer-use"
	TemplateClaudeAgentSDK             = "claude-agent-sdk"
	TemplateYutoriComputerUse          = "yutori-computer-use"
	TemplateMultiProviderComputerUse   = "multi-provider-computer-use"
)

type TemplateInfo struct {
	Name        string
	Description string
	Languages   []string
}

type TemplateKeyValue struct {
	Key   string
	Value string
}

type TemplateKeyValues []TemplateKeyValue

var Templates = map[string]TemplateInfo{
	TemplateSampleApp: {
		Name:        "Sample App",
		Description: "Implements a basic Kernel app",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateCaptchaSolver: {
		Name:        "CAPTCHA Solver",
		Description: "Demo of Kernel's auto-CAPTCHA solving capability",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateAnthropicComputerUse: {
		Name:        "Anthropic Computer Use",
		Description: "Implements an Anthropic computer use agent",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateOpenAIComputerUse: {
		Name:        "OpenAI Computer Use",
		Description: "Implements an OpenAI computer use agent",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateMagnitude: {
		Name:        "Magnitude",
		Description: "Implements the Magnitude.run SDK",
		Languages:   []string{LanguageTypeScript},
	},
	TemplateGeminiComputerUse: {
		Name:        "Gemini Computer Use",
		Description: "Implements a Gemini computer use agent",
		Languages:   []string{LanguageTypeScript},
	},
	TemplateBrowserUse: {
		Name:        "Browser Use",
		Description: "Implements Browser Use SDK",
		Languages:   []string{LanguagePython},
	},
	TemplateStagehand: {
		Name:        "Stagehand",
		Description: "Implements the Stagehand v3 SDK",
		Languages:   []string{LanguageTypeScript},
	},
	TemplateOpenAGIComputerUse: {
		Name:        "OpenAGI Computer Use",
		Description: "Implements an OpenAGI computer use agent",
		Languages:   []string{LanguagePython},
	},
	TemplateClaudeAgentSDK: {
		Name:        "Claude Agent SDK",
		Description: "Implements a Claude Agent SDK browser automation agent",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateYutoriComputerUse: {
		Name:        "Yutori n1 Computer Use",
		Description: "Implements a Yutori n1 computer use agent",
		Languages:   []string{LanguageTypeScript, LanguagePython},
	},
	TemplateMultiProviderComputerUse: {
		Name:        "Multi-Provider Computer Use",
		Description: "Computer use agent with multiple provider support (Anthropic, Gemini)",
		Languages:   []string{LanguageTypeScript},
	},
}

// GetSupportedTemplatesForLanguage returns a list of all supported template names for a given language
func GetSupportedTemplatesForLanguage(language string) TemplateKeyValues {
	templates := make(TemplateKeyValues, 0, len(Templates))
	for tn := range Templates {
		if slices.Contains(Templates[tn].Languages, language) {
			templates = append(templates, TemplateKeyValue{
				Key:   tn,
				Value: fmt.Sprintf("%s - %s", Templates[tn].Name, Templates[tn].Description),
			})
		}
	}

	sort.Slice(templates, func(i, j int) bool {
		// Put computer-use templates first (Anthropic/OpenAI/Gemini), then sort alphabetically.
		priority := func(key string) int {
			switch key {
			case TemplateAnthropicComputerUse:
				return 0
			case TemplateOpenAIComputerUse:
				return 1
			case TemplateGeminiComputerUse:
				return 2
			case TemplateYutoriComputerUse:
				return 3
			default:
				return 10
			}
		}

		pi, pj := priority(templates[i].Key), priority(templates[j].Key)
		if pi != pj {
			return pi < pj
		}
		return templates[i].Key < templates[j].Key
	})

	return templates
}

// GetTemplateDisplayValues extracts display values from TemplateKeyValue slice
func (tkv TemplateKeyValues) GetTemplateDisplayValues() []string {
	options := make([]string, len(tkv))
	for i, kv := range tkv {
		options[i] = kv.Value
	}
	return options
}

// GetTemplateKeyFromValue maps the selected display value back to the template key
func (tkv TemplateKeyValues) GetTemplateKeyFromValue(selectedValue string) (string, error) {
	for _, kv := range tkv {
		if kv.Value == selectedValue {
			return kv.Key, nil
		}
	}
	return "", fmt.Errorf("template not found: %s", selectedValue)
}

// ContainsKey checks if a template key exists in the TemplateKeyValues
func (tkv TemplateKeyValues) ContainsKey(key string) bool {
	for _, kv := range tkv {
		if kv.Key == key {
			return true
		}
	}
	return false
}

type DeployConfig struct {
	EntryPoint    string
	NeedsEnvFile  bool
	InvokeCommand string
}

var Commands = map[string]map[string]DeployConfig{
	LanguageTypeScript: {
		TemplateSampleApp: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  false,
			InvokeCommand: `kernel invoke ts-basic get-page-title --payload '{"url": "https://www.google.com"}'`,
		},
		TemplateCaptchaSolver: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  false,
			InvokeCommand: "kernel invoke ts-captcha-solver test-captcha-solver",
		},
		TemplateStagehand: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-stagehand teamsize-task --payload '{"company": "Kernel"}'`,
		},
		TemplateAnthropicComputerUse: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-anthropic-cua cua-task --payload '{"query": "Navigate to http://magnitasks.com and click on Tasks in the sidebar"}'`,
		},
		TemplateMagnitude: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-magnitude mag-url-extract --payload '{"url": "https://en.wikipedia.org/wiki/Special:Random"}'`,
		},
		TemplateOpenAIComputerUse: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-openai-cua cua-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 5 articles"}'`,
		},
		TemplateGeminiComputerUse: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: "kernel invoke ts-gemini-cua gemini-cua-task",
		},
		TemplateClaudeAgentSDK: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-claude-agent-sdk agent-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 3 stories"}'`,
		},
		TemplateYutoriComputerUse: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'`,
		},
		TemplateMultiProviderComputerUse: {
			EntryPoint:    "index.ts",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke ts-multi-provider-cua cua-task --payload '{"provider": "anthropic", "query": "Navigate to https://example.com and describe the page"}'`,
		},
	},
	LanguagePython: {
		TemplateSampleApp: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  false,
			InvokeCommand: `kernel invoke python-basic get-page-title --payload '{"url": "https://www.google.com"}'`,
		},
		TemplateCaptchaSolver: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  false,
			InvokeCommand: "kernel invoke python-captcha-solver test-captcha-solver",
		},
		TemplateBrowserUse: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke python-bu bu-task --payload '{"task": "Compare the price of gpt-4o and DeepSeek-V3"}'`,
		},
		TemplateAnthropicComputerUse: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke python-anthropic-cua cua-task --payload '{"query": "Navigate to http://magnitasks.com and click on Tasks in the sidebar"}'`,
		},
		TemplateOpenAIComputerUse: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke python-openai-cua cua-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 5 articles"}'`,
		},
		TemplateOpenAGIComputerUse: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke python-openagi-cua openagi-default-task -p '{"instruction": "Navigate to https://agiopen.org and click the What is Computer Use? button", "record_replay": "True"}'`,
		},
		TemplateClaudeAgentSDK: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke py-claude-agent-sdk agent-task --payload '{"task": "Go to https://news.ycombinator.com and get the top 3 stories"}'`,
		},
		TemplateYutoriComputerUse: {
			EntryPoint:    "main.py",
			NeedsEnvFile:  true,
			InvokeCommand: `kernel invoke python-yutori-cua cua-task --payload '{"query": "Navigate to https://example.com and describe the page"}'`,
		},
	},
}

// GetDeployCommand returns the full deploy command string for a given language and template
func GetDeployCommand(language, template string) string {
	langCommands, ok := Commands[language]
	if !ok {
		return ""
	}

	config, ok := langCommands[template]
	if !ok {
		return ""
	}

	cmd := "kernel deploy " + config.EntryPoint
	if config.NeedsEnvFile {
		cmd += " --env-file .env"
	}

	return cmd
}

// GetInvokeSample returns the sample invoke command for a given language and template
func GetInvokeSample(language, template string) string {
	langSamples, ok := Commands[language]
	if !ok {
		return ""
	}

	config, ok := langSamples[template]
	if !ok {
		return ""
	}

	return config.InvokeCommand
}
