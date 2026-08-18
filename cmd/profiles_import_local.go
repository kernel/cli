package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Days               int
	SkipConfirm        bool
	Output             string
	ProjectID          string
	ImportID           string
	Version            string
	WaitTimeout        time.Duration
	PasswordManager    string
	InstallAgentSkills bool
	DashboardLaunch    bool
	ImportHistory      bool
	Project            *kernel.Project
}

type ProfilesImportLocalCmd struct {
	prompter            interactive.Prompter
	homeDir             func() (string, error)
	now                 func() time.Time
	providers           func() []passwordmanager.Provider
	provisioner         managedAuthProvisioner
	managedAuthCapacity func(context.Context) (managedAuthCapacity, error)
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

type cookieImportSelection struct {
	sites []string
	all   bool
}

const managedAuthSiteLimit = 10

const backOption = "← Back"

var errRequestedProjectUnavailable = errors.New("requested Kernel project is not active or available")

var errInteractiveBack = errors.New("go back to the previous browser-import menu")

func dashboardProjectAuthRecovery(err error) bool {
	return errors.Is(err, errRequestedProjectUnavailable) || dashboardProjectUnauthorized(err)
}

func dashboardProjectUnauthorized(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) && apiError.StatusCode == http.StatusUnauthorized
}

func (c ProfilesImportLocalCmd) Run(ctx context.Context, in ProfilesImportLocalInput) (returnErr error) {
	startedAt := time.Now()
	timings := make(map[string]time.Duration)
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("local browser import currently supports macOS")
	}
	if in.Days < 1 || in.Days > 90 {
		return fmt.Errorf("--days must be between 1 and 90")
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
	dashboardHandoff := in.DashboardLaunch && in.ImportID != ""
	var handoffClient *localbrowser.Client
	clientCompletion := localbrowser.ClientCompletion{
		Outcome: "failed", ManagedAuthConnections: make([]localbrowser.ManagedAuthConnection, 0),
	}
	clientFailureStage := "local"
	clientCompletionReported := false
	if dashboardHandoff {
		token, err := auth.BearerToken(ctx)
		if err != nil {
			return err
		}
		handoffClient, err = localbrowser.NewClient(util.GetBaseURL(), token, in.ProjectID)
		if err != nil {
			return err
		}
		defer func() {
			if returnErr == nil || clientCompletionReported {
				return
			}
			reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			status, statusErr := handoffClient.Status(reportCtx, in.ImportID)
			if statusErr == nil && status.Phase == "failed" {
				return
			}
			clientCompletion.Outcome = "failed"
			clientCompletion.Failure = &localbrowser.ClientFailure{Stage: clientFailureStage, Message: "Local browser import did not finish."}
			if _, reportErr := handoffClient.SubmitClientCompletion(reportCtx, in.ImportID, clientCompletion); reportErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("report browser import failure: %w", reportErr))
			}
		}()
	}
	if humanOutput {
		pterm.Info.Println("Looking for local browser profiles...")
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
	explicitSites := in.Sites
	if len(explicitSites) > 0 {
		explicitSites, err = normalizeSites(explicitSites)
		if err != nil {
			return err
		}
	}
	cookieSites := make([]localbrowser.Site, 0)
	if len(explicitSites) == 0 {
		if humanOutput {
			pterm.Info.Println("Finding websites with importable cookies...")
		}
		phaseStarted = time.Now()
		recent, historyErr := localbrowser.RecentSites(ctx, profile, c.now().AddDate(0, 0, -in.Days), 10000)
		timings["history"] = time.Since(phaseStarted)
		if historyErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if humanOutput {
				pterm.Warning.Println("Browser history could not be read; cookie websites will be sorted alphabetically")
			}
			recent = nil
		}
		phaseStarted = time.Now()
		cookieSites, err = localbrowser.CookieSites(ctx, profile, recent)
		timings["cookie_counts"] = time.Since(phaseStarted)
		if err != nil {
			return err
		}
		if len(cookieSites) == 0 {
			return fmt.Errorf("this browser profile has no importable cookies")
		}
	}
	cookieSelection, err := c.chooseCookies(cookieSites, explicitSites, nonInteractive)
	if err != nil {
		return err
	}
	if len(cookieSelection.sites) == 0 {
		return fmt.Errorf("select at least one website")
	}
	since := c.now().AddDate(0, 0, -in.Days)
	profileDataSelection, err := c.chooseLocalProfileData(ctx, profile, since, in.ImportHistory, nonInteractive, humanOutput)
	if err != nil {
		return err
	}
	pendingLogins := pendingManagedAuth{}
	if managedAuthImportRequested(in.PasswordManager, nonInteractive) {
		phaseStarted = time.Now()
		loginSites := rankedManagedAuthSites(cookieSites, cookieSelection.sites, managedAuthSiteLimit)
		availableLoginSites := selectedSiteMetadata(cookieSites, cookieSelection.sites)
		pendingLogins, err = c.chooseManagedAuthLogins(ctx, targetName, loginSites, availableLoginSites, in.PasswordManager, nonInteractive, humanOutput)
		timings["password_manager_discovery"] = time.Since(phaseStarted)
		if err != nil {
			return err
		}
	}
	if !nonInteractive {
		proceed, err := c.confirmBrowserImport(targetName, cookieSelection, cookieSites, profileDataSelection, pendingLogins)
		if err != nil {
			return err
		}
		if !proceed {
			pterm.Info.Println("Browser import canceled; no Kernel resources were changed")
			if dashboardHandoff {
				clientCompletion.Outcome = "canceled"
				clientCompletion.Failure = &localbrowser.ClientFailure{Stage: "local", Message: "Browser import was canceled locally."}
				if _, err := handoffClient.SubmitClientCompletion(ctx, in.ImportID, clientCompletion); err != nil {
					return fmt.Errorf("report browser import cancellation: %w", err)
				}
				clientCompletionReported = true
			}
			return nil
		}
	}
	if humanOutput {
		pterm.Println()
		if cookieSelection.all {
			pterm.Info.Println("Reading all browser cookies...")
		} else {
			pterm.Info.Printf("Reading cookies for %d selected websites...\n", len(cookieSelection.sites))
		}
	}
	exportSites := cookieSelection.sites
	if cookieSelection.all {
		exportSites = nil
	}
	phaseStarted = time.Now()
	cookies, err := localbrowser.ExportCookies(ctx, profile, exportSites)
	timings["cookies"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}
	if len(cookies) == 0 {
		return fmt.Errorf("the selected websites have no importable cookies")
	}
	importedCookieSites := importedCookieSiteCount(cookies)

	version := in.Version
	if version == "" {
		version = "dev"
	}
	phaseStarted = time.Now()
	profileData, itemCounts, err := buildSelectedProfileData(ctx, profile, profileDataSelection, cookies, since)
	if err != nil {
		return err
	}
	fit, err := fitBrowserImportBundle(profileData, itemCounts, func(candidate localbrowser.ProfileData) ([]byte, error) {
		return localbrowser.BuildProfileBundle(ctx, profile, targetName, version, candidate)
	})
	timings["bundle"] = time.Since(phaseStarted)
	if err != nil {
		return err
	}
	if fit.originalSize > 0 {
		if nonInteractive {
			return fmt.Errorf("%w; run interactively to review optional browser data that can be skipped", &localbrowser.BundleTooLargeError{Size: fit.originalSize, Limit: fit.limit})
		}
		proceed, err := c.confirmBundleFallback(fit)
		if err != nil {
			return err
		}
		if !proceed {
			pterm.Info.Println("Browser import canceled; no Kernel resources were changed")
			if dashboardHandoff {
				clientCompletion.Outcome = "canceled"
				clientCompletion.Failure = &localbrowser.ClientFailure{Stage: "local", Message: "Browser import was canceled locally."}
				if _, err := handoffClient.SubmitClientCompletion(ctx, in.ImportID, clientCompletion); err != nil {
					return fmt.Errorf("report browser import cancellation: %w", err)
				}
				clientCompletionReported = true
			}
			return nil
		}
	}
	profileData = fit.data
	itemCounts = fit.itemCounts
	bundle := fit.bundle
	categories := selectedProfileCategories(itemCounts)
	clientCompletion.Counts = localbrowser.ClientCounts{
		Cookies: itemCounts["cookies"], Bookmarks: itemCounts["bookmarks"], History: itemCounts["history"], StorageOrigins: importedStorageOriginCount(profileData.Storage),
	}
	client := handoffClient
	if client == nil {
		token, err := auth.BearerToken(ctx)
		if err != nil {
			return err
		}
		client, err = localbrowser.NewClient(util.GetBaseURL(), token, in.ProjectID)
		if err != nil {
			return err
		}
	}
	var profileSpinner *pterm.SpinnerPrinter
	if humanOutput {
		profileSpinner, _ = pterm.DefaultSpinner.Start(fmt.Sprintf("Creating Kernel profile %q...", targetName))
		defer func() {
			if profileSpinner != nil {
				_ = profileSpinner.Stop()
			}
		}()
	}
	phaseStarted = time.Now()
	importID := in.ImportID
	helperToken := ""
	if dashboardHandoff {
		grant, err := client.AcquireHelperGrant(ctx, importID)
		if err != nil {
			return fmt.Errorf("get scoped browser import grant: %w", err)
		}
		helperToken = grant.HelperToken
	} else {
		created, err := client.Create(ctx)
		if err != nil {
			return err
		}
		importID = created.ID
		helperToken = created.HelperToken
	}
	clientFailureStage = "profile"
	inventory := localbrowser.Inventory{Sources: []localbrowser.Source{{
		ID: profile.ID, Kind: "browser", Name: profile.DisplayName(), Browser: profile.Browser.ID,
		DataTypes: categories, ItemCounts: itemCounts,
	}}}
	status, err := client.SubmitInventory(ctx, importID, helperToken, inventory)
	if err != nil {
		return browserImportProgressError(importID, status.Phase, time.Since(phaseStarted), err)
	}
	if profileSpinner != nil {
		profileSpinner.UpdateText("Preparing browser profile import...")
	}
	selection := localbrowser.Selection{Profiles: []localbrowser.ProfileSelection{{SourceID: profile.ID, TargetName: targetName, Categories: categories}}}
	status, err = client.SubmitSelection(ctx, importID, selection)
	if err != nil {
		return browserImportProgressError(importID, status.Phase, time.Since(phaseStarted), err)
	}
	if profileSpinner != nil {
		profileSpinner.UpdateText("Uploading encrypted browser data...")
	}
	status, err = client.Upload(ctx, importID, helperToken, bundle)
	if err != nil {
		return browserImportProgressError(importID, status.Phase, time.Since(phaseStarted), err)
	}
	if profileSpinner != nil {
		profileSpinner.UpdateText("Applying cookies, bookmarks, history, and website storage (this can take a few minutes)...")
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, in.WaitTimeout)
	defer cancelWait()
	if dashboardHandoff {
		status, err = client.WaitForProfile(waitCtx, importID, 2*time.Second)
	} else {
		status, err = client.Wait(waitCtx, importID, 2*time.Second)
	}
	timings["upload_and_apply"] = time.Since(phaseStarted)
	if err != nil {
		return fmt.Errorf("browser import %s did not complete: %w; check it with: kernel profiles import-status %s", importID, err, importID)
	}
	if status.Applied == nil || len(status.Applied.Profiles) == 0 {
		return fmt.Errorf("browser import completed without a profile")
	}
	appliedProfile := status.Applied.Profiles[0]
	profileID := appliedProfile.ProfileID
	storageSummary := effectiveStorageImportSummary(appliedProfile, itemCounts["storage"], importedStorageOriginCount(profileData.Storage))
	clientCompletion.Counts.StorageOrigins = storageSummary.importedOrigins
	clientFailureStage = "managed_auth"
	if humanOutput {
		if profileSpinner != nil {
			profileSpinner.Success(fmt.Sprintf("Created Kernel profile %q", targetName))
			profileSpinner = nil
		}
		pterm.Success.Printf("Imported %d cookies from %d websites\n", len(cookies), importedCookieSites)
		if count := itemCounts["bookmarks"]; count > 0 {
			pterm.Success.Printf("Imported %d bookmarks\n", count)
		}
		if count := itemCounts["history"]; count > 0 {
			pterm.Success.Printf("Imported %d history entries\n", count)
		}
		if storageSummary.importedEntries > 0 {
			pterm.Success.Printf("Imported %d local storage keys from %d origins\n", storageSummary.importedEntries, storageSummary.importedOrigins)
		}
		if storageSummary.skippedEntries > 0 {
			pterm.Warning.Printf("Skipped %d local storage keys from %d origins that could not be restored\n", storageSummary.skippedEntries, storageSummary.skippedOrigins)
		}
		if profileData.StorageRecordsSkipped > 0 {
			pterm.Warning.Printf("Skipped %d oversized local storage keys from %d origins (1 MiB maximum per key)\n", profileData.StorageRecordsSkipped, profileData.StorageOriginsSkipped)
		}
	}
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
			pterm.Println()
			pterm.Info.Printf("Setting up %d Managed Auth connections...\n", len(approvedLogins))
		}
		phaseStarted = time.Now()
		connectionIDs, err = c.provisioner.Provision(ctx, targetName, approvedLogins)
		clientCompletion.ManagedAuthConnections = managedAuthCompletionConnections(connectionIDs, approvedLogins)
		timings["managed_auth"] = time.Since(phaseStarted)
		if err != nil {
			if humanOutput {
				for _, login := range approvedLogins[:min(len(connectionIDs), len(approvedLogins))] {
					pterm.Success.Println(login.Domain)
				}
			}
			return fmt.Errorf("profile %s is ready and %d Managed Auth connection%s completed, but setup stopped: %w; rerun the import to safely resume", targetName, len(connectionIDs), pluralSuffix(len(connectionIDs)), err)
		}
		if humanOutput {
			for _, login := range approvedLogins {
				pterm.Success.Println(login.Domain)
			}
		}
	}
	installedSkills := 0
	skillWarning := ""
	if len(connectionIDs) > 0 {
		clientFailureStage = "agent_skills"
		phaseStarted = time.Now()
		installedSkills, err = c.offerAgentSkills(home, in.InstallAgentSkills, nonInteractive, humanOutput)
		timings["agent_skills"] = time.Since(phaseStarted)
		if err != nil {
			skillWarning = err.Error()
		}
	}
	if dashboardHandoff {
		clientCompletion.Outcome = "completed"
		clientCompletion.Failure = nil
		if humanOutput {
			pterm.Info.Println("Opening Kernel to finish authentication...")
		}
		if _, err := client.SubmitClientCompletion(ctx, importID, clientCompletion); err != nil {
			return fmt.Errorf("report browser import completion: %w", err)
		}
		clientCompletionReported = true
		ackCtx, cancelAck := context.WithTimeout(ctx, in.WaitTimeout)
		defer cancelAck()
		status, err = client.Wait(ackCtx, importID, 2*time.Second)
		if err != nil {
			return fmt.Errorf("dashboard did not acknowledge browser import %s: %w; reopen Kernel to finish setup", importID, err)
		}
	}
	if in.Output == "json" {
		data, err := json.MarshalIndent(map[string]any{"profile_id": profileID, "profile_name": targetName, "sites": cookieSelection.sites, "cookies_imported": itemCounts["cookies"], "browser_data_imported": itemCounts, "managed_auth_connections": connectionIDs, "agent_skills_installed": installedSkills, "agent_skill_warning": skillWarning, "duration_ms": time.Since(startedAt).Milliseconds(), "timings_ms": durationMilliseconds(timings)}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	pterm.Println()
	pterm.Success.Printf("%s is ready for agents\n", targetName)
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

func managedAuthCompletionConnections(ids []string, records []passwordmanager.Record) []localbrowser.ManagedAuthConnection {
	connections := make([]localbrowser.ManagedAuthConnection, 0, min(len(ids), len(records)))
	for index, id := range ids {
		if index >= len(records) {
			break
		}
		connections = append(connections, localbrowser.ManagedAuthConnection{ID: id, Domain: records[index].Domain})
	}
	return connections
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
		approved, err := c.prompter.ConfirmDefault("install agent skill", "Install the Kernel Managed Auth skill for your local agents?", true)
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

func managedAuthImportRequested(requested string, nonInteractive bool) bool {
	return !strings.EqualFold(strings.TrimSpace(requested), "none") && !(requested == "" && nonInteractive)
}

func importedCookieSiteCount(cookies []localbrowser.Cookie) int {
	sites := make(map[string]struct{})
	for _, cookie := range cookies {
		domain, err := localbrowser.CanonicalSite(cookie.Domain)
		if err == nil {
			sites[domain] = struct{}{}
		}
	}
	return len(sites)
}

func (c ProfilesImportLocalCmd) chooseManagedAuthLogins(ctx context.Context, profileName string, sites []string, availableSites []localbrowser.Site, requested string, nonInteractive, humanOutput bool) (pendingManagedAuth, error) {
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
	var (
		capacityHint    managedAuthCapacity
		capacityHintErr error
		capacityLoaded  bool
	)
	if !nonInteractive && c.managedAuthCapacity != nil {
		capacityHint, capacityHintErr = c.managedAuthCapacity(ctx)
		capacityLoaded = capacityHintErr == nil
	}
	sites, err := c.chooseManagedAuthSites(sites, availableSites, nonInteractive, capacityHint, capacityLoaded && capacityHintErr == nil)
	if err != nil {
		return pendingManagedAuth{}, err
	}
	if len(sites) == 0 {
		return pendingManagedAuth{}, nil
	}
	if humanOutput {
		pterm.Println()
	}
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
				pterm.Println("Enter your Bitwarden master password to find matching logins:")
				if err := authorizer.Authorize(ctx); err != nil {
					if humanOutput {
						pterm.Warning.Printf("Skipping %s: %v\n", provider.Name(), err)
					}
					continue
				}
				if humanOutput {
					pterm.Success.Println("Bitwarden unlocked for this import")
					pterm.Println()
				}
			}
		}
		if humanOutput {
			pterm.Info.Printf("Finding matching %s logins...\n", provider.Name())
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
	if c.provisioner == nil {
		return pendingManagedAuth{}, fmt.Errorf("managed auth importer is unavailable")
	}
	candidates := make([]passwordmanager.Candidate, 0, len(allCandidates))
	for _, sourced := range allCandidates {
		candidates = append(candidates, sourced.candidate)
	}
	existing, err := c.provisioner.Existing(ctx, profileName, candidates)
	if err != nil {
		if failure := managedAuthDiscoveryFailure(requested, "check existing Managed Auth connections", err); failure != nil {
			return pendingManagedAuth{}, failure
		}
		if humanOutput {
			pterm.Warning.Printf("Could not check existing Managed Auth connections; continuing with cookies: %v\n", err)
		}
		return pendingManagedAuth{}, nil
	}
	hasExisting := false
	hasNew := false
	for _, candidate := range candidates {
		if existing[candidateKey(candidate)] {
			hasExisting = true
		} else {
			hasNew = true
		}
	}
	availableConnections := 0
	capacityKnown := !hasNew
	var capacityLookupErr error
	if !hasNew {
		// Existing imports refresh their credential and do not consume quota.
	} else if c.managedAuthCapacity == nil {
		if !hasExisting {
			return pendingManagedAuth{}, fmt.Errorf("managed auth capacity is unavailable")
		}
	} else {
		capacity, capacityErr := capacityHint, capacityHintErr
		if !capacityLoaded {
			capacity, capacityErr = c.managedAuthCapacity(ctx)
		}
		if capacityErr != nil {
			capacityLookupErr = capacityErr
			if hasNew && requested != "" && nonInteractive {
				return pendingManagedAuth{}, fmt.Errorf("check Managed Auth capacity: %w", capacityErr)
			}
			if humanOutput {
				pterm.Warning.Printf("Could not check capacity; only existing Managed Auth connections can be refreshed: %v\n", capacityErr)
			}
		} else {
			capacityKnown = true
			availableConnections = capacity.remaining
			if capacity.unlimited {
				availableConnections = len(sites)
			} else if humanOutput {
				pterm.Info.Printf("Your organization has %d Managed Auth connection slot%s available\n", availableConnections, pluralSuffix(availableConnections))
			}
		}
	}
	if hasNew && !capacityKnown {
		if requested != "" && !hasExisting {
			return pendingManagedAuth{}, fmt.Errorf("check Managed Auth capacity: %w", capacityLookupErr)
		}
		if !hasExisting {
			return pendingManagedAuth{}, nil
		}
	}
	if availableConnections == 0 && hasNew && requested != "" && nonInteractive {
		return pendingManagedAuth{}, fmt.Errorf("your organization has no Managed Auth connection slots available for new logins; delete a connection or upgrade your plan, then retry")
	}
	if capacityKnown && availableConnections == 0 && !hasExisting {
		if requested != "" {
			return pendingManagedAuth{}, fmt.Errorf("your organization has no Managed Auth connection slots available; delete a connection or upgrade your plan, then retry")
		}
		if humanOutput {
			pterm.Info.Println("Managed Auth is at your organization's connection limit; continuing with cookies")
		}
		return pendingManagedAuth{}, nil
	}
	siteRank := make(map[string]int, len(sites))
	for index, site := range sites {
		siteRank[site] = index
	}
	providerRank := make(map[string]int, len(chosenProviders))
	for index, provider := range chosenProviders {
		providerRank[provider.Name()] = index
	}
	sort.SliceStable(allCandidates, func(i, j int) bool {
		left := allCandidates[i]
		right := allCandidates[j]
		if siteRank[left.candidate.Domain] != siteRank[right.candidate.Domain] {
			return siteRank[left.candidate.Domain] < siteRank[right.candidate.Domain]
		}
		if providerRank[left.provider.Name()] != providerRank[right.provider.Name()] {
			return providerRank[left.provider.Name()] < providerRank[right.provider.Name()]
		}
		return left.candidate.Name < right.candidate.Name
	})
	domainCounts := make(map[string]int, len(sites))
	for _, sourced := range allCandidates {
		domainCounts[sourced.candidate.Domain]++
	}
	if requested != "" && nonInteractive {
		requiredNew := 0
		for _, sourced := range allCandidates {
			candidate := sourced.candidate
			if domainCounts[candidate.Domain] == 1 && !existing[candidateKey(candidate)] {
				requiredNew++
			}
		}
		if requiredNew > availableConnections {
			return pendingManagedAuth{}, fmt.Errorf("%d matching logins need new Managed Auth connections, but your organization has %d slot%s available", requiredNew, availableConnections, pluralSuffix(availableConnections))
		}
	}
	if nonInteractive {
		for domain, count := range domainCounts {
			if count > 1 {
				return pendingManagedAuth{}, fmt.Errorf("%s has %d matching logins; rerun interactively to choose one", domain, count)
			}
		}
	}
	approvedCandidates := make([]sourcedPasswordManagerCandidate, 0, len(sites))
	if nonInteractive {
		remaining := availableConnections
		for _, sourced := range allCandidates {
			candidate := sourced.candidate
			if domainCounts[candidate.Domain] != 1 {
				continue
			}
			if !existing[candidateKey(candidate)] {
				if remaining == 0 {
					continue
				}
				remaining--
			}
			approvedCandidates = append(approvedCandidates, sourced)
		}
	} else {
		approvedCandidates, err = c.chooseManagedAuthAccountsByWebsite(sites, allCandidates, existing, availableConnections)
		if err != nil {
			return pendingManagedAuth{}, err
		}
	}
	approvedByProvider := make(map[string][]passwordmanager.Candidate, len(chosenProviders))
	selectedDomains := make(map[string]struct{}, len(approvedCandidates))
	for _, sourced := range approvedCandidates {
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

func (c ProfilesImportLocalCmd) chooseManagedAuthAccountsByWebsite(sites []string, candidates []sourcedPasswordManagerCandidate, existing map[string]bool, availableConnections int) ([]sourcedPasswordManagerCandidate, error) {
	byDomain := make(map[string][]sourcedPasswordManagerCandidate, len(sites))
	for _, candidate := range candidates {
		byDomain[candidate.candidate.Domain] = append(byDomain[candidate.candidate.Domain], candidate)
	}

	pterm.Println()
	pterm.Println("Choose one login per website for Managed Auth:")
	pterm.Println()
	domains := make([]string, 0, len(sites))
	for _, domain := range sites {
		if len(byDomain[domain]) > 0 {
			domains = append(domains, domain)
		}
	}
	choices := make(map[string]sourcedPasswordManagerCandidate, len(domains))
	for domainIndex := 0; domainIndex < len(domains); {
		domain := domains[domainIndex]
		domainCandidates := byDomain[domain]
		if len(domainCandidates) == 1 {
			chosen := domainCandidates[0]
			delete(choices, domain)
			if !existing[candidateKey(chosen.candidate)] && managedAuthNewChoiceCount(choices, existing) >= availableConnections {
				pterm.Warning.Printf("Skipping %s: no Managed Auth connection slots remain\n", domain)
				domainIndex++
				continue
			}
			choices[domain] = chosen
			pterm.Info.Printf("Will use %s — %s %s\n", domain, passwordManagerAbbreviation(chosen.provider.Name()), managedAuthCandidateIdentity(chosen.candidate))
			domainIndex++
			continue
		}
		labels := make([]string, 0, len(domainCandidates))
		byLabel := make(map[string]sourcedPasswordManagerCandidate, len(domainCandidates))
		existingLabels := make([]string, 0, len(domainCandidates))
		labels = groupedLoginLabels(domainCandidates)
		for index, sourced := range domainCandidates {
			label := labels[index]
			byLabel[label] = sourced
			if existing[candidateKey(sourced.candidate)] {
				existingLabels = append(existingLabels, label)
			}
		}
		options := append([]string{}, labels...)
		options = append(options, "Skip this website")
		if previousAmbiguousDomainIndex(domains, byDomain, domainIndex) >= 0 {
			options = append(options, "← Previous website")
		}
		defaultOption := labels[0]
		if previous, ok := choices[domain]; ok {
			for label, candidate := range byLabel {
				if candidateKey(candidate.candidate) == candidateKey(previous.candidate) {
					defaultOption = label
					break
				}
			}
		} else if len(existingLabels) == 1 {
			defaultOption = existingLabels[0]
		} else if managedAuthNewChoiceCount(choices, existing) >= availableConnections {
			defaultOption = "Skip this website"
		}
		selected, err := c.prompter.SelectDefault("login for "+domain, "use arrow keys and press Enter", domain+" — ↑/↓ move, Enter chooses", options, defaultOption)
		if err != nil {
			return nil, err
		}
		switch selected {
		case "← Previous website":
			previousIndex := previousAmbiguousDomainIndex(domains, byDomain, domainIndex)
			if previousIndex < 0 {
				continue
			}
			for index := previousIndex; index < len(domains); index++ {
				delete(choices, domains[index])
			}
			domainIndex = previousIndex
			continue
		case "Skip this website":
			delete(choices, domain)
			domainIndex++
			continue
		}
		chosen := byLabel[selected]
		previous, hadPrevious := choices[domain]
		delete(choices, domain)
		if !existing[candidateKey(chosen.candidate)] && managedAuthNewChoiceCount(choices, existing) >= availableConnections {
			if hadPrevious {
				choices[domain] = previous
			}
			pterm.Warning.Printf("No Managed Auth connection slots remain; choose an existing connection or skip %s\n", domain)
			continue
		}
		choices[domain] = chosen
		domainIndex++
		pterm.Println()
	}
	approved := make([]sourcedPasswordManagerCandidate, 0, len(choices))
	for _, domain := range domains {
		if chosen, ok := choices[domain]; ok {
			approved = append(approved, chosen)
		}
	}
	return approved, nil
}

func previousAmbiguousDomainIndex(domains []string, candidates map[string][]sourcedPasswordManagerCandidate, current int) int {
	for index := current - 1; index >= 0; index-- {
		if len(candidates[domains[index]]) > 1 {
			return index
		}
	}
	return -1
}

func managedAuthNewChoiceCount(choices map[string]sourcedPasswordManagerCandidate, existing map[string]bool) int {
	count := 0
	for _, choice := range choices {
		if !existing[candidateKey(choice.candidate)] {
			count++
		}
	}
	return count
}

func (c ProfilesImportLocalCmd) chooseManagedAuthSites(sites []string, availableSites []localbrowser.Site, nonInteractive bool, capacity managedAuthCapacity, capacityKnown bool) ([]string, error) {
	if nonInteractive || len(sites) == 0 {
		return sites, nil
	}
	prompt, defaultDomains := managedAuthSitePrompt(sites, capacity, capacityKnown)
	recommendedSet := make(map[string]struct{}, len(sites))
	for _, domain := range sites {
		recommendedSet[domain] = struct{}{}
	}
	selected := append([]string(nil), defaultDomains...)
	const findAnotherOption = "+ Find another website"
	for {
		labels, byLabel := managedAuthRecommendationOptions(sites, availableSites)
		for _, domain := range selected {
			if _, recommended := recommendedSet[domain]; recommended {
				continue
			}
			label := managedAuthSiteOption(len(labels), domain, siteMetadata(availableSites, domain))
			labels = append(labels, label)
			byLabel[label] = domain
		}
		labels = append(labels, findAnotherOption)
		currentSet := make(map[string]struct{}, len(selected))
		for _, domain := range selected {
			currentSet[domain] = struct{}{}
		}
		selectedDefaults := make([]string, 0, len(selected))
		for _, label := range labels {
			if _, checked := currentSet[byLabel[label]]; checked {
				selectedDefaults = append(selectedDefaults, label)
			}
		}
		sectionPrompt := prompt + "\nSpace toggles websites · check + Find another website to search · Enter continues"
		selectedLabels, err := c.prompter.MultiSelect("Managed Auth websites", "pass --yes to use the suggested websites", sectionPrompt, labels, selectedDefaults)
		if err != nil {
			return nil, err
		}
		result := make([]string, 0, len(selectedLabels))
		findAnother := false
		for _, label := range selectedLabels {
			if label == findAnotherOption {
				findAnother = true
				continue
			}
			result = append(result, byLabel[label])
		}
		selected = result
		if !findAnother {
			return selected, nil
		}
		options, domains := managedAuthSearchOptions(availableSites, selected)
		if len(options) == 1 {
			pterm.Info.Println("Every browser website is already selected")
			continue
		}
		chosen, err := c.prompter.Select("Managed Auth website", "select a website", "Find another website — type to search, Enter adds, Back returns", options)
		if err != nil {
			return nil, err
		}
		if chosen != backOption && !containsString(selected, domains[chosen]) {
			selected = append(selected, domains[chosen])
		}
	}
}

func managedAuthSitePrompt(sites []string, capacity managedAuthCapacity, capacityKnown bool) (string, []string) {
	defaults := sites
	prompt := "Choose recent websites to find Managed Auth logins"
	if capacityKnown && !capacity.unlimited {
		prompt = fmt.Sprintf("Choose recent websites to find Managed Auth logins (%d new connection slot%s available)", capacity.remaining, pluralSuffix(capacity.remaining))
	}
	return prompt, defaults
}

func managedAuthRecommendationOptions(sites []string, available []localbrowser.Site) ([]string, map[string]string) {
	metadata := make(map[string]localbrowser.Site, len(available))
	for _, site := range available {
		metadata[site.Domain] = site
	}
	options := make([]string, 0, len(sites))
	byOption := make(map[string]string, len(sites))
	for index, domain := range sites {
		label := managedAuthSiteOption(index, domain, metadata[domain])
		options = append(options, label)
		byOption[label] = domain
	}
	return options, byOption
}

func managedAuthSiteOption(index int, domain string, site localbrowser.Site) string {
	label := fmt.Sprintf("%2d  %s", index+1, paddedCompactField(domain, 28))
	if site.Visits > 0 {
		label += fmt.Sprintf("  %9s visits", boundedCount(site.Visits))
	}
	return strings.TrimRight(label, " ")
}

func siteMetadata(sites []localbrowser.Site, domain string) localbrowser.Site {
	for _, site := range sites {
		if site.Domain == domain {
			return site
		}
	}
	return localbrowser.Site{Domain: domain}
}

func managedAuthSearchOptions(available []localbrowser.Site, selected []string) ([]string, map[string]string) {
	options := make([]string, 0, len(available)+1)
	options = append(options, backOption)
	byOption := make(map[string]string, len(available))
	for index, site := range available {
		if containsString(selected, site.Domain) {
			continue
		}
		label := fmt.Sprintf("%d  %-28s %s visits", index+1, compactField(site.Domain, 28), boundedCount(site.Visits))
		label = ansi.Truncate(label, 64, "…")
		options = append(options, label)
		byOption[label] = site.Domain
	}
	return options, byOption
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func managedAuthDiscoveryFailure(requested, action string, err error) error {
	if requested == "" {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
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

func (c ProfilesImportLocalCmd) chooseSites(recent []localbrowser.Site, requested []string, skipPrompt bool) ([]string, error) {
	if len(requested) > 0 {
		return normalizeSites(requested)
	}
	defaults := make([]string, 0, len(recent))
	for _, site := range recent {
		defaults = append(defaults, site.Domain)
	}
	if skipPrompt {
		return defaults, nil
	}
	selected := append([]localbrowser.Site(nil), recent...)
	for {
		options, byOption := cookieRemovalOptions(selected)
		chosen, err := c.prompter.Select("websites", "pass --sites or --yes", "Remove a website, or continue", options)
		if err != nil {
			return nil, err
		}
		if chosen == backOption {
			return nil, errInteractiveBack
		}
		removeIndex, remove := byOption[chosen]
		if !remove {
			break
		}
		selected = append(selected[:removeIndex], selected[removeIndex+1:]...)
	}
	result := make([]string, 0, len(selected))
	for _, site := range selected {
		result = append(result, site.Domain)
	}
	return result, nil
}

func cookieRemovalOptions(sites []localbrowser.Site) ([]string, map[string]int) {
	options := make([]string, 0, len(sites)+2)
	options = append(options, fmt.Sprintf("Done — import %d website%s", len(sites), pluralSuffix(len(sites))))
	options = append(options, backOption)
	byOption := make(map[string]int, len(sites))
	for index, site := range sites {
		label := cookieSiteLabel(index, site)
		options = append(options, label)
		byOption[label] = index
	}
	return options, byOption
}

func (c ProfilesImportLocalCmd) chooseCookies(sites []localbrowser.Site, requested []string, skipPrompt bool) (cookieImportSelection, error) {
	if len(requested) > 0 {
		normalized, err := normalizeSites(requested)
		return cookieImportSelection{sites: normalized}, err
	}
	allSites := make([]string, 0, len(sites))
	for _, site := range sites {
		allSites = append(allSites, site.Domain)
	}
	if skipPrompt {
		return cookieImportSelection{sites: allSites, all: true}, nil
	}
	options := cookieImportOptions(sites)
	for {
		choice, err := c.prompter.Select("cookies", "pass --sites or --yes", "What should Kernel import?", options)
		if err != nil {
			return cookieImportSelection{}, err
		}
		if choice == options[0] {
			return cookieImportSelection{sites: allSites, all: true}, nil
		}
		selected, err := c.chooseSites(sites, nil, false)
		if errors.Is(err, errInteractiveBack) {
			continue
		}
		return cookieImportSelection{sites: selected}, err
	}
}

func cookieImportOptions(sites []localbrowser.Site) []string {
	total := 0
	for _, site := range sites {
		total += site.CookieCount
	}
	all := fmt.Sprintf(
		"All cookies (recommended) — %d cookie%s across %d website%s",
		total,
		pluralSuffix(total),
		len(sites),
		pluralSuffix(len(sites)),
	)
	return []string{all, "Choose websites"}
}

func rankedManagedAuthSites(ranked []localbrowser.Site, selected []string, limit int) []string {
	if len(ranked) == 0 {
		return append([]string(nil), selected[:min(limit, len(selected))]...)
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, site := range selected {
		selectedSet[site] = struct{}{}
	}
	result := make([]string, 0, min(limit, len(selected)))
	for _, site := range ranked {
		if site.Visits == 0 {
			continue
		}
		if _, ok := selectedSet[site.Domain]; !ok {
			continue
		}
		result = append(result, site.Domain)
		if len(result) == limit {
			break
		}
	}
	if len(result) == 0 {
		return append([]string(nil), selected[:min(limit, len(selected))]...)
	}
	return result
}

func selectedSiteMetadata(ranked []localbrowser.Site, selected []string) []localbrowser.Site {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, domain := range selected {
		selectedSet[domain] = struct{}{}
	}
	result := make([]localbrowser.Site, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, site := range ranked {
		if _, ok := selectedSet[site.Domain]; !ok {
			continue
		}
		result = append(result, site)
		seen[site.Domain] = struct{}{}
	}
	for _, domain := range selected {
		if _, ok := seen[domain]; !ok {
			result = append(result, localbrowser.Site{Domain: domain})
		}
	}
	return result
}

func cookieSiteLabel(index int, site localbrowser.Site) string {
	label := fmt.Sprintf("%d  %-24s %s visits · %s cookies", index+1, compactField(site.Domain, 24), boundedCount(site.Visits), boundedCount(site.CookieCount))
	return ansi.Truncate(label, 64, "…")
}

func boundedCount(value int) string {
	text := fmt.Sprintf("%d", value)
	if len(text) <= 9 {
		return text
	}
	return fmt.Sprintf("%.2e", float64(value))
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

func paddedCompactField(value string, width int) string {
	value = compactField(value, width)
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}

func groupedLoginCandidateLabel(provider string, candidate passwordmanager.Candidate) string {
	idSuffix := candidate.ID
	if len(idSuffix) > 6 {
		idSuffix = idSuffix[len(idSuffix)-6:]
	}
	return groupedLoginCandidateLabelWithID(provider, candidate, idSuffix)
}

func managedAuthCandidateIdentity(candidate passwordmanager.Candidate) string {
	if candidate.Username != "" {
		return compactField(candidate.Username, 40)
	}
	return compactField(candidate.Name, 40)
}

func groupedLoginLabels(candidates []sourcedPasswordManagerCandidate) []string {
	baseCounts := make(map[string]int, len(candidates))
	for _, sourced := range candidates {
		baseCounts[groupedLoginCandidateLabel(sourced.provider.Name(), sourced.candidate)]++
	}
	labels := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for index, sourced := range candidates {
		base := groupedLoginCandidateLabel(sourced.provider.Name(), sourced.candidate)
		label := base
		if baseCounts[base] > 1 {
			label = groupedLoginCandidateLabelWithID(sourced.provider.Name(), sourced.candidate, sourced.candidate.ID)
			if _, collision := seen[label]; collision {
				label += fmt.Sprintf(" · #%d", index+1)
			}
		}
		labels = append(labels, label)
		seen[label] = struct{}{}
	}
	return labels
}

func groupedLoginCandidateLabelWithID(provider string, candidate passwordmanager.Candidate, id string) string {
	identity := candidate.Username
	if identity == "" {
		identity = "no username"
	}
	return fmt.Sprintf("%s  %-24s  %s · %s", passwordManagerAbbreviation(provider), compactField(identity, 24), compactField(candidate.Name, 22), compactField(id, 12))
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

func chooseImportProject(ctx context.Context, projects ProjectListService, prompter interactive.Prompter, requested string, nonInteractive bool) (kernel.Project, error) {
	const pageSize int64 = 100
	active := make([]kernel.Project, 0)
	for offset := int64(0); ; {
		page, err := projects.List(ctx, kernel.ProjectListParams{Limit: param.NewOpt(pageSize), Offset: param.NewOpt(offset)})
		if err != nil {
			return kernel.Project{}, fmt.Errorf("list Kernel projects: %w", err)
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
	if requested != "" {
		for _, project := range active {
			if project.ID == requested {
				return project, nil
			}
		}
		return kernel.Project{}, fmt.Errorf("%w: %q", errRequestedProjectUnavailable, requested)
	}
	if len(active) == 0 {
		return kernel.Project{}, fmt.Errorf("no active Kernel projects were found")
	}
	if len(active) == 1 {
		return active[0], nil
	}
	if nonInteractive {
		return kernel.Project{}, fmt.Errorf("multiple Kernel projects are available; pass --project")
	}
	options := make([]string, 0, len(active))
	byOption := make(map[string]kernel.Project, len(active))
	for index, project := range active {
		label := projectOptionLabel(index, project)
		options = append(options, label)
		byOption[label] = project
	}
	chosen, err := prompter.Select("Kernel project", "pass --project", "Choose the Kernel project for this import", options)
	if err != nil {
		return kernel.Project{}, err
	}
	return byOption[chosen], nil
}

var profileIDNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var cuidLikeProfileName = regexp.MustCompile(`^[a-z0-9]{24}$`)

var profilesImportLocalCmd = &cobra.Command{
	Use:   "import-local",
	Short: "Import a local browser profile",
	Long:  "Import cookies and selected portable data from a local Google Chrome or Helium profile on macOS into a Kernel browser profile.",
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
	profilesImportLocalCmd.Flags().StringSlice("sites", nil, "Import cookies only for these website domains")
	profilesImportLocalCmd.Flags().Int("days", 30, "Rank websites used during the last number of days (1-90)")
	profilesImportLocalCmd.Flags().Duration("wait-timeout", 30*time.Minute, "Maximum time to wait for the import to complete")
	profilesImportLocalCmd.Flags().BoolP("yes", "y", false, "Import all cookies and use unambiguous defaults without prompting")
	profilesImportLocalCmd.Flags().Bool("history", true, "Include browsing history from the selected --days window")
	profilesImportLocalCmd.Flags().String("password-manager", "", "Password managers to search: bitwarden, 1password, both comma-separated, all, or none")
	profilesImportLocalCmd.Flags().Bool("install-agent-skills", false, "Install the Kernel Managed Auth skill into detected agent directories")
	addJSONOutputFlag(profilesImportLocalCmd)
	addJSONOutputFlag(profilesImportStatusCmd)
}

func runProfilesImportLocal(cmd *cobra.Command, _ []string) error {
	browserProfile, _ := cmd.Flags().GetString("browser-profile")
	profileName, _ := cmd.Flags().GetString("profile-name")
	sites, _ := cmd.Flags().GetStringSlice("sites")
	days, _ := cmd.Flags().GetInt("days")
	waitTimeout, _ := cmd.Flags().GetDuration("wait-timeout")
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	passwordManager, _ := cmd.Flags().GetString("password-manager")
	installAgentSkills, _ := cmd.Flags().GetBool("install-agent-skills")
	importHistory, _ := cmd.Flags().GetBool("history")
	output, _ := cmd.Flags().GetString("output")
	project, _ := cmd.Flags().GetString("project")
	return runProfilesImportLocalWithInput(cmd, ProfilesImportLocalInput{
		BrowserProfile: browserProfile, ProfileName: profileName, Sites: sites, Days: days,
		SkipConfirm: skipConfirm, Output: output, ProjectID: resolveProjectSelection(project), Version: metadata.Version,
		WaitTimeout: waitTimeout, PasswordManager: passwordManager, InstallAgentSkills: installAgentSkills, ImportHistory: importHistory,
	})
}

func runProfilesImportLocalWithInput(cmd *cobra.Command, input ProfilesImportLocalInput) error {
	client := getKernelClient(cmd)
	var project kernel.Project
	if input.Project != nil {
		project = *input.Project
	} else {
		var err error
		project, err = chooseImportProject(cmd.Context(), &client.Projects, interactive.NewPrompter(), input.ProjectID, input.SkipConfirm || input.Output == "json")
		if err != nil {
			if input.DashboardLaunch && dashboardProjectAuthRecovery(err) {
				return fmt.Errorf("validate dashboard project: %w; if this is the wrong account, run `kernel login --force` (and unset KERNEL_API_KEY if set), then open the import again", err)
			}
			return err
		}
	}
	if input.DashboardLaunch && input.Output != "json" {
		pterm.Success.Printf("Connected to Kernel project %s\n", compactField(project.Name, 48))
	}
	projectClient, err := auth.GetAuthenticatedClient(
		option.WithProjectID(project.ID),
		option.WithHeader("X-Kernel-Cli-Version", metadata.Version),
	)
	if err != nil {
		return fmt.Errorf("create project-scoped client: %w", err)
	}
	credentials := projectClient.Credentials
	connections := projectClient.Auth.Connections
	limits := projectClient.Organization.Limits
	c := ProfilesImportLocalCmd{
		prompter:            interactive.NewPrompter(),
		providers:           passwordmanager.Detect,
		provisioner:         kernelManagedAuthProvisioner{credentials: &credentials, connections: &connections},
		managedAuthCapacity: func(ctx context.Context) (managedAuthCapacity, error) { return loadManagedAuthCapacity(ctx, &limits) },
	}
	input.ProjectID = project.ID
	if input.Version == "" {
		input.Version = metadata.Version
	}
	return c.Run(cmd.Context(), input)
}
