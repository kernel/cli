package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/kernel/cli/internal/agentskills"
	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/kernel/cli/internal/passwordmanager"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type ProfilesImportLocalInput struct {
	BrowserProfile     string
	ProfileName        string
	Sites              []string
	Count              int
	Days               int
	SkipConfirm        bool
	Output             string
	ProjectID          string
	Version            string
	WaitTimeout        time.Duration
	PasswordManager    string
	InstallAgentSkills bool
}

type ProfilesImportLocalCmd struct {
	prompter    interactive.Prompter
	homeDir     func() (string, error)
	now         func() time.Time
	providers   func() []passwordmanager.Provider
	provisioner managedAuthProvisioner
}

type pendingManagedAuth struct {
	providers []pendingProviderLogins
}

type pendingProviderLogins struct {
	provider   passwordmanager.Provider
	candidates []passwordmanager.Candidate
}

type sourcedPasswordManagerCandidate struct {
	provider  passwordmanager.Provider
	candidate passwordmanager.Candidate
}

func (c ProfilesImportLocalCmd) Run(ctx context.Context, in ProfilesImportLocalInput) error {
	startedAt := time.Now()
	timings := make(map[string]time.Duration)
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("local browser import currently supports macOS")
	}
	if in.Count < 5 || in.Count > 10 {
		return fmt.Errorf("--count must be between 5 and 10")
	}
	if in.Days < 1 || in.Days > 90 {
		return fmt.Errorf("--days must be between 1 and 90")
	}
	if len(in.Sites) > 10 {
		return fmt.Errorf("select at most 10 sites")
	}
	if in.WaitTimeout <= 0 {
		in.WaitTimeout = 30 * time.Minute
	}
	if c.homeDir == nil {
		c.homeDir = os.UserHomeDir
	}
	if c.now == nil {
		c.now = time.Now
	}

	home, err := c.homeDir()
	if err != nil {
		return err
	}
	humanOutput := in.Output != "json"
	nonInteractive := in.SkipConfirm || !humanOutput
	if humanOutput {
		pterm.Info.Println("Looking for local Chrome profiles...")
	}
	phaseStarted := time.Now()
	profiles, err := localbrowser.DiscoverMacOSProfiles(home)
	timings["discovery"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no Google Chrome or Helium profiles were found")
	}
	profile, err := c.chooseProfile(profiles, in.BrowserProfile)
	if err != nil {
		return err
	}

	if humanOutput {
		pterm.Success.Printf("Found %s\n", profile.DisplayName())
	}
	targetName := in.ProfileName
	if targetName == "" {
		targetName = defaultImportedProfileName(profile)
	}
	if targetName == "" || len(targetName) > 255 || profileIDNameCharacters.MatchString(targetName) || cuidLikeProfileName.MatchString(targetName) {
		return fmt.Errorf("profile name must be 1-255 letters, numbers, dots, underscores, or hyphens and cannot be a cuid-like string")
	}
	recent := make([]localbrowser.Site, 0)
	if len(in.Sites) == 0 {
		phaseStarted = time.Now()
		recent, err = localbrowser.RecentSites(ctx, profile, c.now().AddDate(0, 0, -in.Days), 10)
		timings["history"] = time.Since(phaseStarted)
		if err != nil {
			return err
		}
		if len(recent) == 0 {
			return fmt.Errorf("no websites were visited in this profile during the last %d days", in.Days)
		}
	}
	selected, err := c.chooseSites(recent, in.Sites, in.Count, nonInteractive)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("select at least one website")
	}
	phaseStarted = time.Now()
	pendingLogins, err := c.chooseManagedAuthLogins(ctx, selected, in.PasswordManager, nonInteractive, humanOutput)
	timings["password_manager_discovery"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}

	if humanOutput {
		pterm.Info.Printf("Reading cookies for %d selected websites...\n", len(selected))
	}
	phaseStarted = time.Now()
	cookies, err := localbrowser.ExportCookies(ctx, profile, selected)
	timings["cookies"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}
	if len(cookies) == 0 {
		return fmt.Errorf("the selected websites have no importable cookies")
	}
	selectedSites := selectedSiteDetails(recent, selected)
	selectedSites = localbrowser.CountCookiesBySite(cookies, selectedSites)
	if humanOutput {
		printSelectedSites(selectedSites)
	}

	if !nonInteractive {
		ok, err := c.prompter.Confirm("import browser data", fmt.Sprintf("Import %d cookies into Kernel profile %q?", len(cookies), targetName))
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Import cancelled")
			return nil
		}
	}

	version := in.Version
	if version == "" {
		version = "dev"
	}
	phaseStarted = time.Now()
	bundle, err := localbrowser.BuildCookieBundle(ctx, profile, targetName, version, cookies)
	timings["bundle"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}
	token, err := auth.BearerToken(ctx)
	if err != nil {
		return err
	}
	client, err := localbrowser.NewClient(util.GetBaseURL(), token, in.ProjectID)
	if err != nil {
		return err
	}
	if humanOutput {
		pterm.Info.Println("Creating Kernel profile...")
	}
	phaseStarted = time.Now()
	created, err := client.Create(ctx)
	if err != nil {
		return err
	}
	inventory := localbrowser.Inventory{Sources: []localbrowser.Source{{
		ID: profile.ID, Kind: "browser", Name: profile.DisplayName(), Browser: profile.Browser.ID,
		DataTypes: []string{"cookies"}, ItemCounts: map[string]int{"cookies": len(cookies)},
	}}}
	status, err := client.SubmitInventory(ctx, created.ID, created.HelperToken, inventory)
	if err != nil {
		return browserImportProgressError(created.ID, status.Phase, time.Since(phaseStarted), err)
	}
	selection := localbrowser.Selection{Profiles: []localbrowser.ProfileSelection{{SourceID: profile.ID, TargetName: targetName, Categories: []string{"cookies"}}}, CredentialSources: make([]string, 0)}
	status, err = client.SubmitSelection(ctx, created.ID, selection)
	if err != nil {
		return browserImportProgressError(created.ID, status.Phase, time.Since(phaseStarted), err)
	}
	status, err = client.Upload(ctx, created.ID, created.HelperToken, bundle)
	if err != nil {
		return browserImportProgressError(created.ID, status.Phase, time.Since(phaseStarted), err)
	}
	if humanOutput {
		pterm.Info.Println("Saving browser state to Kernel...")
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, in.WaitTimeout)
	defer cancelWait()
	status, err = client.Wait(waitCtx, created.ID, 2*time.Second)
	timings["upload_and_apply"] = time.Since(phaseStarted)
	if err != nil {
		return fmt.Errorf("browser import %s did not complete: %w; check it with: kernel profiles import-status %s", created.ID, err, created.ID)
	}
	if status.Applied == nil || len(status.Applied.Profiles) == 0 {
		return fmt.Errorf("browser import completed without a profile")
	}
	profileID := status.Applied.Profiles[0].ProfileID
	connectionIDs := make([]string, 0)
	approvedLogins := make([]passwordmanager.Record, 0)
	if len(pendingLogins.providers) > 0 {
		phaseStarted = time.Now()
		for _, pendingProvider := range pendingLogins.providers {
			records, revealErr := pendingProvider.provider.Reveal(ctx, pendingProvider.candidates)
			if revealErr != nil {
				return fmt.Errorf("profile %s is ready, but approved %s items could not be read: %w", targetName, pendingProvider.provider.Name(), revealErr)
			}
			approvedLogins = append(approvedLogins, records...)
		}
		timings["password_manager_reveal"] = time.Since(phaseStarted)
	}
	if len(approvedLogins) > 0 {
		if c.provisioner == nil {
			return fmt.Errorf("managed auth importer is unavailable")
		}
		if humanOutput {
			pterm.Info.Printf("Creating %d Managed Auth connections...\n", len(approvedLogins))
		}
		phaseStarted = time.Now()
		connectionIDs, err = c.provisioner.Provision(ctx, targetName, approvedLogins)
		timings["managed_auth"] = time.Since(phaseStarted)
		if err != nil {
			return fmt.Errorf("profile %s is ready, but Managed Auth setup stopped: %w", targetName, err)
		}
	}
	installedSkills := 0
	skillWarning := ""
	if len(connectionIDs) > 0 {
		phaseStarted = time.Now()
		installedSkills, err = c.offerAgentSkills(home, in.InstallAgentSkills, nonInteractive, humanOutput)
		timings["agent_skills"] = time.Since(phaseStarted)
		if err != nil {
			skillWarning = err.Error()
		}
	}
	if in.Output == "json" {
		data, err := json.MarshalIndent(map[string]any{"profile_id": profileID, "profile_name": targetName, "sites": selected, "cookies_imported": len(cookies), "managed_auth_connections": connectionIDs, "agent_skills_installed": installedSkills, "agent_skill_warning": skillWarning, "duration_ms": time.Since(startedAt).Milliseconds(), "timings_ms": durationMilliseconds(timings)}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	pterm.Success.Printf("%s is ready for agents\n", targetName)
	pterm.Printf("Imported %d cookies from %d websites\n", len(cookies), len(selected))
	if len(connectionIDs) > 0 {
		pterm.Printf("Created %d Managed Auth connections with credentials and supported TOTP secrets\n", len(connectionIDs))
	}
	if skillWarning != "" {
		pterm.Warning.Printf("Managed Auth is ready, but agent skill installation was skipped: %s\n", skillWarning)
	}
	pterm.Printf("Completed in %s (local read %s, upload and apply %s)\n", time.Since(startedAt).Round(time.Millisecond), (timings["history"] + timings["cookies"]).Round(time.Millisecond), timings["upload_and_apply"].Round(time.Millisecond))
	if len(connectionIDs) > 0 {
		pterm.Printf("Password manager %s, Managed Auth %s\n", (timings["password_manager_discovery"] + timings["password_manager_reveal"]).Round(time.Millisecond), timings["managed_auth"].Round(time.Millisecond))
	}
	pterm.Printf("Next: kernel browsers create --profile %s\n", targetName)
	return nil
}

func (c ProfilesImportLocalCmd) offerAgentSkills(home string, installRequested, nonInteractive, humanOutput bool) (int, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	targets := agentskills.Detect(workingDirectory, home)
	if len(targets) == 0 {
		return 0, nil
	}
	if !installRequested {
		if nonInteractive {
			return 0, nil
		}
		approved, err := c.prompter.Confirm("install agent skill", "Install the Kernel Managed Auth skill for your local agents?")
		if err != nil || !approved {
			return 0, err
		}
	}
	labels := make([]string, 0, len(targets))
	byLabel := make(map[string]agentskills.Target, len(targets))
	for _, target := range targets {
		label := fmt.Sprintf("%-10s %s", target.Agent, target.Path)
		labels = append(labels, label)
		byLabel[label] = target
	}
	selectedLabels := labels
	if !nonInteractive {
		selectedLabels, err = c.prompter.MultiSelect("agent skills", "pass --install-agent-skills", "Choose agents that should learn Managed Auth", labels, labels)
		if err != nil {
			return 0, err
		}
	}
	selected := make([]agentskills.Target, 0, len(selectedLabels))
	for _, label := range selectedLabels {
		selected = append(selected, byLabel[label])
	}
	if err := agentskills.Install(selected); err != nil {
		return 0, err
	}
	if humanOutput {
		pterm.Success.Printf("Installed the Kernel Managed Auth skill for %d agent environments\n", len(selected))
	}
	return len(selected), nil
}

func (c ProfilesImportLocalCmd) chooseManagedAuthLogins(ctx context.Context, sites []string, requested string, nonInteractive, humanOutput bool) (pendingManagedAuth, error) {
	if strings.EqualFold(strings.TrimSpace(requested), "none") || (requested == "" && nonInteractive) {
		return pendingManagedAuth{}, nil
	}
	if c.providers == nil {
		c.providers = passwordmanager.Detect
	}
	providers := c.providers()
	if len(providers) == 0 {
		if requested != "" {
			return pendingManagedAuth{}, fmt.Errorf("no supported password-manager CLI was found; install `bw` or `op`, then retry")
		}
		if humanOutput {
			pterm.Info.Println("No Bitwarden or 1Password CLI found; continuing with cookies")
		}
		return pendingManagedAuth{}, nil
	}
	chosenProviders := make([]passwordmanager.Provider, 0, len(providers))
	if requested == "" {
		options := make([]string, 0, len(providers))
		byName := make(map[string]passwordmanager.Provider, len(providers))
		for _, provider := range providers {
			options = append(options, provider.Name())
			byName[provider.Name()] = provider
		}
		selected, err := c.prompter.MultiSelect("password managers", "pass --password-manager", "Choose password managers to search", options, options)
		if err != nil {
			return pendingManagedAuth{}, err
		}
		for _, name := range selected {
			chosenProviders = append(chosenProviders, byName[name])
		}
	} else {
		requestedNames := strings.Split(requested, ",")
		if strings.EqualFold(strings.TrimSpace(requested), "all") {
			chosenProviders = append(chosenProviders, providers...)
		} else {
			for _, name := range requestedNames {
				provider := findPasswordManager(providers, strings.TrimSpace(name))
				if provider == nil {
					return pendingManagedAuth{}, fmt.Errorf("password manager %q is unavailable; use bitwarden, 1password, all, or none", strings.TrimSpace(name))
				}
				chosenProviders = append(chosenProviders, provider)
			}
		}
	}
	if len(chosenProviders) == 0 {
		return pendingManagedAuth{}, nil
	}
	chosenProviders = deduplicatePasswordManagers(chosenProviders)
	allCandidates := make([]sourcedPasswordManagerCandidate, 0)
	for _, provider := range chosenProviders {
		if authorizer, ok := provider.(passwordmanager.InteractiveAuthorizer); ok {
			required, err := authorizer.AuthorizationRequired(ctx)
			if err != nil {
				if nonInteractive {
					return pendingManagedAuth{}, err
				}
				if humanOutput {
					pterm.Warning.Printf("Skipping %s: %v\n", provider.Name(), err)
				}
				continue
			}
			if required {
				if nonInteractive {
					return pendingManagedAuth{}, fmt.Errorf("Bitwarden is locked; unlock it with `export BW_SESSION=$(bw unlock --raw)`, then retry")
				}
				approved, err := c.prompter.Confirm("unlock Bitwarden", "Bitwarden is locked. Unlock it locally to find matching logins?")
				if err != nil {
					return pendingManagedAuth{}, err
				}
				if !approved {
					continue
				}
				if err := authorizer.Authorize(ctx); err != nil {
					if humanOutput {
						pterm.Warning.Printf("Skipping %s: %v\n", provider.Name(), err)
					}
					continue
				}
				if humanOutput {
					pterm.Success.Println("Bitwarden unlocked for this import")
				}
			}
		}
		if humanOutput {
			pterm.Info.Printf("Find matching %s logins...\n", provider.Name())
		}
		candidates, err := discoverProviderCandidates(ctx, provider, sites, nonInteractive, humanOutput)
		if err != nil {
			return pendingManagedAuth{}, err
		}
		if len(candidates) == 0 && humanOutput {
			pterm.Info.Printf("No %s logins matched the selected websites\n", provider.Name())
		}
		for _, candidate := range candidates {
			allCandidates = append(allCandidates, sourcedPasswordManagerCandidate{provider: provider, candidate: candidate})
		}
	}
	if len(allCandidates) == 0 {
		return pendingManagedAuth{}, nil
	}
	labels := make([]string, 0, len(allCandidates))
	byLabel := make(map[string]sourcedPasswordManagerCandidate, len(allCandidates))
	defaultLabels := make([]string, 0, len(sites))
	domainCounts := make(map[string]int, len(sites))
	for _, sourced := range allCandidates {
		domainCounts[sourced.candidate.Domain]++
	}
	for index, sourced := range allCandidates {
		candidate := sourced.candidate
		idSuffix := candidate.ID
		if len(idSuffix) > 6 {
			idSuffix = idSuffix[len(idSuffix)-6:]
		}
		label := loginCandidateLabel(index, sourced.provider.Name(), candidate, idSuffix)
		labels = append(labels, label)
		byLabel[label] = sourced
		if domainCounts[candidate.Domain] == 1 {
			defaultLabels = append(defaultLabels, label)
		}
	}
	if nonInteractive {
		for domain, count := range domainCounts {
			if count > 1 {
				return pendingManagedAuth{}, fmt.Errorf("%s has %d matching logins; rerun interactively to choose one", domain, count)
			}
		}
	}
	chosenLabels := defaultLabels
	if !nonInteractive {
		for {
			selected, promptErr := c.prompter.MultiSelect("logins", "pass --yes to approve suggested matches", "Choose at most one login per website", labels, chosenLabels)
			if promptErr != nil {
				return pendingManagedAuth{}, promptErr
			}
			if duplicateDomain := duplicateSelectedDomain(selected, byLabel); duplicateDomain != "" {
				pterm.Warning.Printf("Choose only one login for %s; your selections are preserved\n", duplicateDomain)
				chosenLabels = selected
				continue
			}
			chosenLabels = selected
			break
		}
	}
	approvedByProvider := make(map[string][]passwordmanager.Candidate, len(chosenProviders))
	selectedDomains := make(map[string]struct{}, len(chosenLabels))
	for _, label := range chosenLabels {
		sourced := byLabel[label]
		candidate := sourced.candidate
		if _, exists := selectedDomains[candidate.Domain]; exists {
			return pendingManagedAuth{}, fmt.Errorf("select at most one login for %s", candidate.Domain)
		}
		selectedDomains[candidate.Domain] = struct{}{}
		approvedByProvider[sourced.provider.Name()] = append(approvedByProvider[sourced.provider.Name()], candidate)
	}
	pending := pendingManagedAuth{providers: make([]pendingProviderLogins, 0, len(approvedByProvider))}
	for _, provider := range chosenProviders {
		if candidates := approvedByProvider[provider.Name()]; len(candidates) > 0 {
			pending.providers = append(pending.providers, pendingProviderLogins{provider: provider, candidates: candidates})
		}
	}
	return pending, nil
}

func discoverProviderCandidates(ctx context.Context, provider passwordmanager.Provider, sites []string, nonInteractive, humanOutput bool) ([]passwordmanager.Candidate, error) {
	candidates, err := provider.Candidates(ctx, sites)
	if err == nil {
		return candidates, nil
	}
	if nonInteractive {
		return nil, err
	}
	if humanOutput {
		pterm.Warning.Printf("Skipping %s: %v\n", provider.Name(), err)
	}
	return nil, nil
}

func duplicateSelectedDomain(selected []string, candidates map[string]sourcedPasswordManagerCandidate) string {
	seen := make(map[string]struct{}, len(selected))
	for _, label := range selected {
		domain := candidates[label].candidate.Domain
		if _, exists := seen[domain]; exists {
			return domain
		}
		seen[domain] = struct{}{}
	}
	return ""
}

func findPasswordManager(providers []passwordmanager.Provider, requested string) passwordmanager.Provider {
	for _, provider := range providers {
		if strings.EqualFold(requested, strings.ToLower(strings.ReplaceAll(provider.Name(), "Password", "password"))) ||
			(strings.EqualFold(requested, "1password") && provider.Name() == "1Password") ||
			(strings.EqualFold(requested, "bitwarden") && provider.Name() == "Bitwarden") {
			return provider
		}
	}
	return nil
}

func deduplicatePasswordManagers(providers []passwordmanager.Provider) []passwordmanager.Provider {
	result := make([]passwordmanager.Provider, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		key := strings.ToLower(provider.Name())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, provider)
	}
	return result
}

func browserImportProgressError(importID, phase string, elapsed time.Duration, err error) error {
	if phase == "" {
		phase = "unknown"
	}
	return fmt.Errorf("browser import %s stopped after %s in phase %s: %w; check it with: kernel profiles import-status %s", importID, elapsed.Round(time.Millisecond), phase, err, importID)
}

func durationMilliseconds(values map[string]time.Duration) map[string]int64 {
	result := make(map[string]int64, len(values))
	for name, duration := range values {
		result[name] = duration.Milliseconds()
	}
	return result
}

func runProfilesImportStatus(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	if err := validateJSONOutput(output); err != nil {
		return err
	}
	token, err := auth.BearerToken(cmd.Context())
	if err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString("project")
	client, err := localbrowser.NewClient(util.GetBaseURL(), token, resolveProjectSelection(project))
	if err != nil {
		return err
	}
	status, err := client.Status(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	if output == "json" {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		if status.Phase == "failed" {
			return fmt.Errorf("browser import %s failed", status.ID)
		}
		return nil
	}
	pterm.Info.Printf("Browser import %s: %s\n", status.ID, status.Phase)
	if status.Applied != nil {
		for _, applied := range status.Applied.Profiles {
			pterm.Success.Printf("Profile %s is ready (%s)\n", applied.TargetName, applied.ProfileID)
		}
		if status.Applied.Failure != nil {
			return fmt.Errorf("%s: %s", status.Applied.Failure.Stage, status.Applied.Failure.Message)
		}
	}
	if status.Phase == "failed" {
		return fmt.Errorf("browser import %s failed", status.ID)
	}
	return nil
}

func (c ProfilesImportLocalCmd) chooseProfile(profiles []localbrowser.Profile, requested string) (localbrowser.Profile, error) {
	if requested != "" {
		matches := make([]localbrowser.Profile, 0, 1)
		for _, profile := range profiles {
			if profile.ID == requested || strings.EqualFold(profile.DisplayName(), requested) {
				matches = append(matches, profile)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return localbrowser.Profile{}, fmt.Errorf("browser profile %q is ambiguous; use its profile ID", requested)
		}
		return localbrowser.Profile{}, fmt.Errorf("browser profile %q was not found", requested)
	}
	if len(profiles) == 1 {
		return profiles[0], nil
	}
	options := make([]string, 0, len(profiles))
	byOption := make(map[string]localbrowser.Profile, len(profiles))
	for _, profile := range profiles {
		label := profile.DisplayName()
		if profile.Directory != "" {
			label += " (" + profile.Directory + ")"
		}
		options = append(options, label)
		byOption[label] = profile
	}
	chosen, err := c.prompter.Select("browser profile", "pass --browser-profile", "Choose the local browser profile to import", options)
	if err != nil {
		return localbrowser.Profile{}, err
	}
	if profile, ok := byOption[chosen]; ok {
		return profile, nil
	}
	return localbrowser.Profile{}, fmt.Errorf("selected browser profile is unavailable")
}

func (c ProfilesImportLocalCmd) chooseSites(recent []localbrowser.Site, requested []string, count int, skipPrompt bool) ([]string, error) {
	if len(requested) > 0 {
		return normalizeSites(requested)
	}
	if len(recent) < count {
		count = len(recent)
	}
	defaults := make([]string, 0, count)
	for _, site := range recent[:count] {
		defaults = append(defaults, site.Domain)
	}
	if skipPrompt {
		return defaults, nil
	}
	labels := make([]string, 0, len(recent))
	byLabel := make(map[string]string, len(recent))
	defaultLabels := make([]string, 0, count)
	for index, site := range recent {
		label := fmt.Sprintf("%-32s %d visits", site.Domain, site.Visits)
		labels = append(labels, label)
		byLabel[label] = site.Domain
		if index < count {
			defaultLabels = append(defaultLabels, label)
		}
	}
	chosen, err := c.prompter.MultiSelect("websites", "pass --sites or --yes", "Choose websites to bring to Kernel", labels, defaultLabels)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(chosen))
	for _, label := range chosen {
		result = append(result, byLabel[label])
	}
	return result, nil
}

func normalizeSites(values []string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			domain, err := localbrowser.CanonicalSite(part)
			if err != nil {
				return nil, err
			}
			if domain == "" {
				continue
			}
			if _, ok := seen[domain]; ok {
				continue
			}
			seen[domain] = struct{}{}
			result = append(result, domain)
		}
	}
	sort.Strings(result)
	return result, nil
}

func selectedSiteDetails(recent []localbrowser.Site, selected []string) []localbrowser.Site {
	byDomain := make(map[string]localbrowser.Site, len(recent))
	for _, site := range recent {
		byDomain[site.Domain] = site
	}
	result := make([]localbrowser.Site, 0, len(selected))
	for _, domain := range selected {
		site := byDomain[domain]
		site.Domain = domain
		result = append(result, site)
	}
	return result
}

func printSelectedSites(sites []localbrowser.Site) {
	rows := pterm.TableData{{"Website", "Recent visits", "Cookies"}}
	for _, site := range sites {
		rows = append(rows, []string{site.Domain, fmt.Sprintf("%d", site.Visits), fmt.Sprintf("%d", site.CookieCount)})
	}
	PrintTableNoPad(rows, true)
}

func defaultImportedProfileName(profile localbrowser.Profile) string {
	name := strings.ToLower(profile.Browser.ID + "-" + profile.Name)
	name = strings.Trim(profileIDNameCharacters.ReplaceAllString(name, "-"), "-")
	if name == "" {
		return "imported-browser"
	}
	return name
}

func compactField(value string, limit int) string {
	value = ansi.Strip(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, value)
	return ansi.Truncate(strings.Join(strings.Fields(value), " "), limit, "…")
}

func loginCandidateLabel(index int, provider string, candidate passwordmanager.Candidate, idSuffix string) string {
	identity := candidate.Username
	if identity == "" {
		identity = "no username"
	}
	indexLabel := fmt.Sprintf("%d", index+1)
	indexWidth := max(2, ansi.StringWidth(indexLabel))
	nameWidth := max(8, 16-(indexWidth-2))
	return fmt.Sprintf("%*s  %s  %s  %s  %s · %s", indexWidth, indexLabel, passwordManagerAbbreviation(provider), compactField(candidate.Domain, 15), compactField(identity, 18), compactField(candidate.Name, nameWidth), idSuffix)
}

func passwordManagerAbbreviation(provider string) string {
	if strings.EqualFold(provider, "Bitwarden") {
		return "BW"
	}
	if strings.EqualFold(provider, "1Password") {
		return "1P"
	}
	return compactField(provider, 2)
}

func projectOptionLabel(index int, project kernel.Project) string {
	return fmt.Sprintf("%2d  %s", index+1, compactField(project.Name, 48))
}

func chooseImportProject(ctx context.Context, projects ProjectListService, prompter interactive.Prompter, requested string, nonInteractive bool) (string, error) {
	if requested != "" {
		return requested, nil
	}
	const pageSize int64 = 100
	active := make([]kernel.Project, 0)
	for offset := int64(0); ; {
		page, err := projects.List(ctx, kernel.ProjectListParams{Limit: param.NewOpt(pageSize), Offset: param.NewOpt(offset)})
		if err != nil {
			return "", fmt.Errorf("list Kernel projects: %w", err)
		}
		if page == nil || len(page.Items) == 0 {
			break
		}
		for _, project := range page.Items {
			if project.Status == kernel.ProjectStatusActive {
				active = append(active, project)
			}
		}
		if int64(len(page.Items)) < pageSize {
			break
		}
		offset += int64(len(page.Items))
	}
	if len(active) == 0 {
		return "", fmt.Errorf("no active Kernel projects were found")
	}
	if len(active) == 1 {
		return active[0].ID, nil
	}
	if nonInteractive {
		return "", fmt.Errorf("multiple Kernel projects are available; pass --project")
	}
	options := make([]string, 0, len(active))
	byOption := make(map[string]string, len(active))
	for index, project := range active {
		label := projectOptionLabel(index, project)
		options = append(options, label)
		byOption[label] = project.ID
	}
	chosen, err := prompter.Select("Kernel project", "pass --project", "Choose the Kernel project for this import", options)
	if err != nil {
		return "", err
	}
	return byOption[chosen], nil
}

var profileIDNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var cuidLikeProfileName = regexp.MustCompile(`^[a-z0-9]{24}$`)

var profilesImportLocalCmd = &cobra.Command{
	Use:   "import-local",
	Short: "Import recent websites from a local browser",
	Long:  "Import selected cookies from a local Google Chrome or Helium profile on macOS into a Kernel browser profile.",
	Args:  cobra.NoArgs,
	RunE:  runProfilesImportLocal,
}

var profilesImportStatusCmd = &cobra.Command{
	Use:   "import-status <import-id>",
	Short: "Check a local browser import",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesImportStatus,
}

func init() {
	profilesCmd.AddCommand(profilesImportLocalCmd)
	profilesCmd.AddCommand(profilesImportStatusCmd)
	profilesImportLocalCmd.Flags().String("browser-profile", "", "Local browser profile ID or name")
	profilesImportLocalCmd.Flags().String("profile-name", "", "Kernel profile name")
	profilesImportLocalCmd.Flags().StringSlice("sites", nil, "Website domains to import (up to 10)")
	profilesImportLocalCmd.Flags().Int("count", 5, "Number of recent websites selected by default (5-10)")
	profilesImportLocalCmd.Flags().Int("days", 30, "Rank websites used during the last number of days (1-90)")
	profilesImportLocalCmd.Flags().Duration("wait-timeout", 30*time.Minute, "Maximum time to wait for the import to complete")
	profilesImportLocalCmd.Flags().BoolP("yes", "y", false, "Use suggested websites and skip confirmation")
	profilesImportLocalCmd.Flags().String("password-manager", "", "Password managers to search: bitwarden, 1password, both comma-separated, all, or none")
	profilesImportLocalCmd.Flags().Bool("install-agent-skills", false, "Install the Kernel Managed Auth skill into detected agent directories")
	addJSONOutputFlag(profilesImportLocalCmd)
	addJSONOutputFlag(profilesImportStatusCmd)
}

func runProfilesImportLocal(cmd *cobra.Command, _ []string) error {
	browserProfile, _ := cmd.Flags().GetString("browser-profile")
	profileName, _ := cmd.Flags().GetString("profile-name")
	sites, _ := cmd.Flags().GetStringSlice("sites")
	count, _ := cmd.Flags().GetInt("count")
	days, _ := cmd.Flags().GetInt("days")
	waitTimeout, _ := cmd.Flags().GetDuration("wait-timeout")
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	passwordManager, _ := cmd.Flags().GetString("password-manager")
	installAgentSkills, _ := cmd.Flags().GetBool("install-agent-skills")
	output, _ := cmd.Flags().GetString("output")
	project, _ := cmd.Flags().GetString("project")
	project = resolveProjectSelection(project)
	client := getKernelClient(cmd)
	project, err := chooseImportProject(cmd.Context(), &client.Projects, interactive.NewPrompter(), project, skipConfirm || output == "json")
	if err != nil {
		return err
	}
	projectClient, err := auth.GetAuthenticatedClient(
		option.WithProjectID(project),
		option.WithHeader("X-Kernel-Cli-Version", metadata.Version),
	)
	if err != nil {
		return fmt.Errorf("create project-scoped client: %w", err)
	}
	credentials := projectClient.Credentials
	connections := projectClient.Auth.Connections
	c := ProfilesImportLocalCmd{prompter: interactive.NewPrompter(), providers: passwordmanager.Detect, provisioner: kernelManagedAuthProvisioner{credentials: &credentials, connections: &connections}}
	return c.Run(cmd.Context(), ProfilesImportLocalInput{BrowserProfile: browserProfile, ProfileName: profileName, Sites: sites, Count: count, Days: days, SkipConfirm: skipConfirm, Output: output, ProjectID: project, Version: metadata.Version, WaitTimeout: waitTimeout, PasswordManager: passwordManager, InstallAgentSkills: installAgentSkills})
}
