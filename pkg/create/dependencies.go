package create

import (
	"fmt"
	"os/exec"

	"github.com/pterm/pterm"
)

// InstallDependencies sets up project dependencies based on language
func InstallDependencies(appPath string, ci CreateInput) (string, error) {
	language := ci.Language
	template := ci.Template
	appName := ci.Name

	installCommand, ok := InstallCommands[language]
	if !ok {
		return "", fmt.Errorf("unsupported language: %s", language)
	}

	requiredTool := RequiredTools[language]
	if requiredTool != "" && !RequiredTools.CheckToolAvailable(language) {
		return getNextStepsWithToolInstall(appName, language, requiredTool, template), nil
	}

	spinner, _ := pterm.DefaultSpinner.Start(pterm.Sprintf("Setting up %s environment...", language))

	cmd := exec.Command("sh", "-c", installCommand)
	cmd.Dir = appPath

	if err := cmd.Run(); err != nil {
		spinner.Stop()
		pterm.Warning.Println("Failed to install dependencies. Please install them manually:")
		switch language {
		case LanguageTypeScript:
			pterm.Printfln("  cd %s", appName)
			pterm.Printfln("  pnpm install")
		case LanguagePython:
			pterm.Printfln("  cd %s", appName)
			pterm.Println("  uv venv && source .venv/bin/activate && uv sync")
		}
		pterm.Println()
		return getNextStepsStandard(appName, language, template), nil
	}

	spinner.Success(pterm.Sprintf("✔ %s environment set up successfully", language))

	return getNextStepsStandard(appName, language, template), nil
}

// getNextStepsWithToolInstall returns next steps message including tool installation
func getNextStepsWithToolInstall(appName string, language string, requiredTool string, template string) string {
	deployCommand := GetDeployCommand(language, template)
	invokeCommand := GetInvokeSample(language, template)

	pterm.Warning.Printfln(" %s is not installed or not in PATH", requiredTool)

	switch language {
	case LanguageTypeScript:
		return pterm.FgYellow.Sprintf(`Next steps:
  # Install pnpm (choose one):
  npm install -g pnpm
  # or: brew install pnpm
  # or: curl -fsSL https://get.pnpm.io/install.sh | sh -

  # Then install dependencies:
  cd %s
  pnpm install

  # Deploy your app:
  kernel login  # or: export KERNEL_API_KEY=<YOUR_API_KEY>
  %s
  %s
`, appName, deployCommand, invokeCommand)
	case LanguagePython:
		return pterm.FgYellow.Sprintf(`Next steps:
  # Install uv (choose one):
  curl -LsSf https://astral.sh/uv/install.sh | sh
  # or: brew install uv
  # or: pipx install uv

  # Then set up your environment:
  cd %s
  uv venv && source .venv/bin/activate && uv sync

  # Deploy your app:
  kernel login  # or: export KERNEL_API_KEY=<YOUR_API_KEY>
  %s
  %s
`, appName, deployCommand, invokeCommand)
	default:
		return ""
	}
}

// getNextStepsStandard returns standard next steps message
func getNextStepsStandard(appName string, language string, template string) string {
	deployCommand := GetDeployCommand(language, template)
	invokeCommand := GetInvokeSample(language, template)
	return pterm.FgYellow.Sprintf(`Next steps:
  cd %s
  kernel login  # or: export KERNEL_API_KEY=<YOUR_API_KEY>
  %s
  %s
`, appName, deployCommand, invokeCommand)
}
