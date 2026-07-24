package cmd

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/klauspost/compress/zstd"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// ProfilesService defines the subset of the Kernel SDK profile client that we use.
type ProfilesService interface {
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.Profile, err error)
	List(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.Profile], err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
	New(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (res *kernel.Profile, err error)
	Download(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *http.Response, err error)
}

type ProfilesGetInput struct {
	Identifier string
	Output     string
}

type ProfilesListInput struct {
	Output  string
	Page    int
	PerPage int
	Query   string
}

type ProfilesCreateInput struct {
	Name   string
	Output string
}

type ProfilesDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type ProfilesDownloadInput struct {
	Identifier string
	To         string
}

// ProfilesCmd handles profile operations independent of cobra.
type ProfilesCmd struct {
	profiles ProfilesService
}

func (p ProfilesCmd) List(ctx context.Context, in ProfilesListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	page := in.Page
	perPage := in.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	if in.Output != "json" {
		pterm.Info.Println("Fetching profiles...")
	}

	params := kernel.ProfileListParams{}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
	}
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	result, err := p.profiles.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.Profile
	if result != nil {
		items = result.Items
	}

	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	if in.Output == "json" {
		if len(items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No profiles found")
		return nil
	}
	rows := pterm.TableData{{"Profile ID", "Name", "Created At", "Updated At", "Last Used At"}}
	for _, prof := range items {
		name := prof.Name
		if name == "" {
			name = "-"
		}
		rows = append(rows, []string{
			prof.ID,
			name,
			util.FormatLocal(prof.CreatedAt),
			util.FormatLocal(prof.UpdatedAt),
			util.FormatLocal(prof.LastUsedAt),
		})
	}
	PrintTableNoPad(rows, true)

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		nextPage := page + 1
		nextCmd := fmt.Sprintf("kernel profile list --page %d --per-page %d", nextPage, perPage)
		if in.Query != "" {
			nextCmd += fmt.Sprintf(" --query \"%s\"", in.Query)
		}
		pterm.Printf("Next: %s\n", nextCmd)
	}

	return nil
}

func (p ProfilesCmd) Get(ctx context.Context, in ProfilesGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	item, err := p.profiles.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if item == nil || item.ID == "" {
		if in.Output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Error.Printf("Profile '%s' not found\n", in.Identifier)
		return nil
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	name := item.Name
	if name == "" {
		name = "-"
	}
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", item.ID})
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Created At", util.FormatLocal(item.CreatedAt)})
	rows = append(rows, []string{"Updated At", util.FormatLocal(item.UpdatedAt)})
	rows = append(rows, []string{"Last Used At", util.FormatLocal(item.LastUsedAt)})
	PrintTableNoPad(rows, true)
	return nil
}

func (p ProfilesCmd) Create(ctx context.Context, in ProfilesCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.ProfileNewParams{}
	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}
	item, err := p.profiles.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	name := item.Name
	if name == "" {
		name = "-"
	}
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", item.ID})
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Created At", util.FormatLocal(item.CreatedAt)})
	rows = append(rows, []string{"Last Used At", util.FormatLocal(item.LastUsedAt)})
	PrintTableNoPad(rows, true)
	return nil
}

func (p ProfilesCmd) Delete(ctx context.Context, in ProfilesDeleteInput) error {
	// Resolve using Get first; treat not found as success with a message
	item, err := p.profiles.Get(ctx, in.Identifier)
	if err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Profile '%s' not found\n", in.Identifier)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	if item == nil || item.ID == "" {
		pterm.Info.Printf("Profile '%s' not found\n", in.Identifier)
		return nil
	}

	if !in.SkipConfirm {
		if !interactive.IsInteractive() {
			return interactive.ErrConfirmationRequired(fmt.Sprintf("delete profile '%s'", in.Identifier))
		}
		msg := fmt.Sprintf("Are you sure you want to delete profile '%s'?", in.Identifier)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := p.profiles.Delete(ctx, in.Identifier); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Profile '%s' not found\n", in.Identifier)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted profile: %s\n", in.Identifier)
	return nil
}

func (p ProfilesCmd) Download(ctx context.Context, in ProfilesDownloadInput) error {
	if in.To == "" {
		return fmt.Errorf("missing required --to <path> for extraction directory")
	}

	res, err := p.profiles.Download(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, res.Body)
		pterm.Info.Printf("Profile '%s' has no saved data yet. Use it in a browser session first to capture state.\n", in.Identifier)
		return nil
	}

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("unexpected status %d from profile download: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := extractProfileArchive(res.Body, in.To); err != nil {
		return fmt.Errorf("extract profile archive: %w", err)
	}

	pterm.Success.Printf("Extracted profile '%s' to %s\n", in.Identifier, in.To)
	return nil
}

// extractProfileArchive streams a zstd-compressed tar archive into destDir.
// Files and directories are created relative to destDir; symlinks and other
// special entry types are skipped. Path-traversal entries are rejected.
func extractProfileArchive(r io.Reader, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	cleanedDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}

	decoder, err := zstd.NewReader(r)
	if err != nil {
		return fmt.Errorf("zstd init: %w", err)
	}
	defer decoder.Close()

	tr := tar.NewReader(decoder)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		destPath := filepath.Join(cleanedDest, header.Name)
		if !strings.HasPrefix(destPath, cleanedDest+string(os.PathSeparator)) && destPath != cleanedDest {
			return fmt.Errorf("illegal entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", destPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return fmt.Errorf("mkdir parent of %s: %w", destPath, err)
			}
			f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return fmt.Errorf("create %s: %w", destPath, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("write %s: %w", destPath, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", destPath, err)
			}
		}
	}
	return nil
}

// --- Cobra wiring ---

var profilesCmd = &cobra.Command{
	Use:     "profiles",
	Aliases: []string{"profile"},
	Short:   "Manage profiles",
	Long:    "Commands for managing Kernel browser profiles",
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List profiles",
	Args:  cobra.NoArgs,
	RunE:  runProfilesList,
}

var profilesGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get a profile by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesGet,
}

var profilesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new profile",
	Args:  cobra.NoArgs,
	RunE:  runProfilesCreate,
}

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a profile by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesDelete,
}

var profilesDownloadCmd = &cobra.Command{
	Use:   "download <id-or-name> --to <dir>",
	Short: "Download a profile and extract it to a directory",
	Long:  "Download a profile and extract its zstd-compressed user-data tar archive into the directory given by --to. The directory is created if it does not exist.",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesDownload,
}

func init() {
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesGetCmd)
	profilesCmd.AddCommand(profilesCreateCmd)
	profilesCmd.AddCommand(profilesDeleteCmd)
	profilesCmd.AddCommand(profilesDownloadCmd)

	addJSONOutputFlag(profilesListCmd)
	profilesListCmd.Flags().Int("per-page", 20, "Items per page (default 20)")
	profilesListCmd.Flags().Int("page", 1, "Page number (1-based)")
	profilesListCmd.Flags().String("query", "", "Search profiles by name or ID")
	addJSONOutputFlag(profilesGetCmd)
	addJSONOutputFlag(profilesCreateCmd)
	profilesCreateCmd.Flags().String("name", "", "Optional unique profile name")
	profilesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	profilesDownloadCmd.Flags().String("to", "", "Directory to extract the profile into (required)")
	_ = profilesDownloadCmd.MarkFlagRequired("to")
}

func runProfilesList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	perPage, _ := cmd.Flags().GetInt("per-page")
	page, _ := cmd.Flags().GetInt("page")
	query, _ := cmd.Flags().GetString("query")

	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.List(cmd.Context(), ProfilesListInput{
		Output:  output,
		Page:    page,
		PerPage: perPage,
		Query:   query,
	})
}

func runProfilesGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Get(cmd.Context(), ProfilesGetInput{Identifier: args[0], Output: output})
}

func runProfilesCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	name, _ := cmd.Flags().GetString("name")
	output, _ := cmd.Flags().GetString("output")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Create(cmd.Context(), ProfilesCreateInput{Name: name, Output: output})
}

func runProfilesDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Delete(cmd.Context(), ProfilesDeleteInput{Identifier: args[0], SkipConfirm: skip})
}

func runProfilesDownload(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	to, _ := cmd.Flags().GetString("to")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Download(cmd.Context(), ProfilesDownloadInput{Identifier: args[0], To: to})
}
