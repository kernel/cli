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

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type ProfilesImportLocalInput struct {
	BrowserProfile string
	ProfileName    string
	Sites          []string
	Count          int
	Days           int
	SkipConfirm    bool
	Output         string
	ProjectID      string
	Version        string
	WaitTimeout    time.Duration
}

type ProfilesImportLocalCmd struct {
	prompter interactive.Prompter
	homeDir  func() (string, error)
	now      func() time.Time
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
	selected, err := c.chooseSites(recent, in.Sites, in.Count, in.SkipConfirm)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("select at least one website")
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

	if !in.SkipConfirm {
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
	if in.Output == "json" {
		data, err := json.MarshalIndent(map[string]any{"profile_id": profileID, "profile_name": targetName, "sites": selected, "cookies_imported": len(cookies), "duration_ms": time.Since(startedAt).Milliseconds(), "timings_ms": durationMilliseconds(timings)}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	pterm.Success.Printf("%s is ready for agents\n", targetName)
	pterm.Printf("Imported %d cookies from %d websites\n", len(cookies), len(selected))
	pterm.Printf("Completed in %s (local read %s, upload and apply %s)\n", time.Since(startedAt).Round(time.Millisecond), (timings["history"] + timings["cookies"]).Round(time.Millisecond), timings["upload_and_apply"].Round(time.Millisecond))
	pterm.Printf("Next: kernel browsers create --profile %s\n", targetName)
	return nil
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
	output, _ := cmd.Flags().GetString("output")
	project, _ := cmd.Flags().GetString("project")
	project = resolveProjectSelection(project)
	c := ProfilesImportLocalCmd{prompter: interactive.NewPrompter()}
	return c.Run(cmd.Context(), ProfilesImportLocalInput{BrowserProfile: browserProfile, ProfileName: profileName, Sites: sites, Count: count, Days: days, SkipConfirm: skipConfirm, Output: output, ProjectID: project, Version: metadata.Version, WaitTimeout: waitTimeout})
}
