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
}

type ProfilesImportLocalCmd struct {
	prompter interactive.Prompter
	homeDir  func() (string, error)
	now      func() time.Time
}

func (c ProfilesImportLocalCmd) Run(ctx context.Context, in ProfilesImportLocalInput) error {
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
	profiles, err := localbrowser.DiscoverMacOSProfiles(home)
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
	recent, err := localbrowser.RecentSites(ctx, profile, c.now().AddDate(0, 0, -in.Days), 10)
	if err != nil {
		return err
	}
	if len(recent) == 0 && len(in.Sites) == 0 {
		return fmt.Errorf("no websites were visited in this profile during the last %d days", in.Days)
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
	cookies, err := localbrowser.ExportCookies(ctx, profile, selected)
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

	targetName := in.ProfileName
	if targetName == "" {
		targetName = defaultImportedProfileName(profile)
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
	bundle, err := localbrowser.BuildCookieBundle(ctx, profile, targetName, version, cookies)
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
	created, err := client.Create(ctx)
	if err != nil {
		return err
	}
	inventory := localbrowser.Inventory{Sources: []localbrowser.Source{{
		ID: profile.ID, Kind: "browser", Name: profile.DisplayName(), Browser: profile.Browser.ID,
		DataTypes: []string{"cookies"}, ItemCounts: map[string]int{"cookies": len(cookies)},
	}}}
	if _, err := client.SubmitInventory(ctx, created.ID, created.HelperToken, inventory); err != nil {
		return err
	}
	selection := localbrowser.Selection{Profiles: []localbrowser.ProfileSelection{{SourceID: profile.ID, TargetName: targetName, Categories: []string{"cookies"}}}, CredentialSources: make([]string, 0)}
	if _, err := client.SubmitSelection(ctx, created.ID, selection); err != nil {
		return err
	}
	if _, err := client.Upload(ctx, created.ID, created.HelperToken, bundle); err != nil {
		return err
	}
	if humanOutput {
		pterm.Info.Println("Saving browser state to Kernel...")
	}
	status, err := client.Wait(ctx, created.ID, 2*time.Second)
	if err != nil {
		return err
	}
	if status.Applied == nil || len(status.Applied.Profiles) == 0 {
		return fmt.Errorf("browser import completed without a profile")
	}
	profileID := status.Applied.Profiles[0].ProfileID
	if in.Output == "json" {
		data, err := json.MarshalIndent(map[string]any{"profile_id": profileID, "profile_name": targetName, "sites": selected, "cookies_imported": len(cookies)}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	pterm.Success.Printf("%s is ready for agents\n", targetName)
	pterm.Printf("Imported %d cookies from %d websites\n", len(cookies), len(selected))
	pterm.Printf("Next: kernel browsers create --profile %s\n", targetName)
	return nil
}

func (c ProfilesImportLocalCmd) chooseProfile(profiles []localbrowser.Profile, requested string) (localbrowser.Profile, error) {
	if requested != "" {
		for _, profile := range profiles {
			if profile.ID == requested || strings.EqualFold(profile.DisplayName(), requested) {
				return profile, nil
			}
		}
		return localbrowser.Profile{}, fmt.Errorf("browser profile %q was not found", requested)
	}
	if len(profiles) == 1 {
		return profiles[0], nil
	}
	options := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		options = append(options, profile.DisplayName())
	}
	chosen, err := c.prompter.Select("browser profile", "pass --browser-profile", "Choose the local browser profile to import", options)
	if err != nil {
		return localbrowser.Profile{}, err
	}
	for _, profile := range profiles {
		if profile.DisplayName() == chosen {
			return profile, nil
		}
	}
	return localbrowser.Profile{}, fmt.Errorf("selected browser profile is unavailable")
}

func (c ProfilesImportLocalCmd) chooseSites(recent []localbrowser.Site, requested []string, count int, skipPrompt bool) ([]string, error) {
	if len(requested) > 0 {
		return normalizeSites(requested), nil
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

func normalizeSites(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			domain := strings.TrimSpace(strings.ToLower(part))
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
	return result
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

var profilesImportLocalCmd = &cobra.Command{
	Use:   "import-local",
	Short: "Import recent websites from a local browser",
	Long:  "Import selected cookies from a local Google Chrome or Helium profile on macOS into a Kernel browser profile.",
	Args:  cobra.NoArgs,
	RunE:  runProfilesImportLocal,
}

func init() {
	profilesCmd.AddCommand(profilesImportLocalCmd)
	profilesImportLocalCmd.Flags().String("browser-profile", "", "Local browser profile ID or name")
	profilesImportLocalCmd.Flags().String("profile-name", "", "Kernel profile name")
	profilesImportLocalCmd.Flags().StringSlice("sites", nil, "Website domains to import (up to 10)")
	profilesImportLocalCmd.Flags().Int("count", 5, "Number of recent websites selected by default (5-10)")
	profilesImportLocalCmd.Flags().Int("days", 30, "Rank websites used during the last number of days (1-90)")
	profilesImportLocalCmd.Flags().BoolP("yes", "y", false, "Use suggested websites and skip confirmation")
	addJSONOutputFlag(profilesImportLocalCmd)
}

func runProfilesImportLocal(cmd *cobra.Command, _ []string) error {
	browserProfile, _ := cmd.Flags().GetString("browser-profile")
	profileName, _ := cmd.Flags().GetString("profile-name")
	sites, _ := cmd.Flags().GetStringSlice("sites")
	count, _ := cmd.Flags().GetInt("count")
	days, _ := cmd.Flags().GetInt("days")
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	output, _ := cmd.Flags().GetString("output")
	project, _ := cmd.Flags().GetString("project")
	project = resolveProjectSelection(project)
	c := ProfilesImportLocalCmd{prompter: interactive.NewPrompter()}
	return c.Run(cmd.Context(), ProfilesImportLocalInput{BrowserProfile: browserProfile, ProfileName: profileName, Sites: sites, Count: count, Days: days, SkipConfirm: skipConfirm, Output: output, ProjectID: project, Version: metadata.Version})
}
