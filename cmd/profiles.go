package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// ProfilesService defines the subset of the Kernel SDK profile client that we use.
type ProfilesService interface {
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.Profile, err error)
	List(ctx context.Context, opts ...option.RequestOption) (res *[]kernel.Profile, err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
	New(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (res *kernel.Profile, err error)
	Download(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *http.Response, err error)
}

type ProfilesGetInput struct {
	Identifier string
}

type ProfilesCreateInput struct {
	Name string
}

type ProfilesDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type ProfilesDownloadInput struct {
	Identifier string
	Output     string
	Pretty     bool
}

// ProfilesCmd handles profile operations independent of cobra.
type ProfilesCmd struct {
	profiles ProfilesService
}

func (p ProfilesCmd) List(ctx context.Context) error {
	pterm.Info.Println("Fetching profiles...")
	items, err := p.profiles.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if items == nil || len(*items) == 0 {
		pterm.Info.Println("No profiles found")
		return nil
	}
	rows := pterm.TableData{{"Profile ID", "Name", "Created At", "Updated At", "Last Used At"}}
	for _, prof := range *items {
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
	return nil
}

func (p ProfilesCmd) Get(ctx context.Context, in ProfilesGetInput) error {
	item, err := p.profiles.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if item == nil || item.ID == "" {
		pterm.Error.Printf("Profile '%s' not found\n", in.Identifier)
		return nil
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
	params := kernel.ProfileNewParams{}
	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}
	item, err := p.profiles.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
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
	res, err := p.profiles.Download(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()

	if in.Output == "" {
		pterm.Error.Println("Missing --to output file path")
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}

	f, err := os.Create(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to create file: %v\n", err)
		return nil
	}
	defer f.Close()
	if in.Pretty {
		var buf bytes.Buffer
		body, _ := io.ReadAll(res.Body)
		if len(body) == 0 {
			pterm.Error.Println("Empty response body")
			return nil
		}
		if err := json.Indent(&buf, body, "", "  "); err != nil {
			pterm.Error.Printf("Failed to pretty-print JSON: %v\n", err)
			return nil
		}
		if _, err := io.Copy(f, &buf); err != nil {
			pterm.Error.Printf("Failed to write pretty-printed JSON: %v\n", err)
			return nil
		}
		return nil
	} else {
		if _, err := io.Copy(f, res.Body); err != nil {
			pterm.Error.Printf("Failed to write file: %v\n", err)
			return nil
		}
	}

	pterm.Success.Printf("Saved profile to %s\n", in.Output)
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
	Use:   "download <id-or-name>",
	Short: "Download a profile as a ZIP archive",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesDownload,
}

func init() {
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesGetCmd)
	profilesCmd.AddCommand(profilesCreateCmd)
	profilesCmd.AddCommand(profilesDeleteCmd)
	profilesCmd.AddCommand(profilesDownloadCmd)

	profilesCreateCmd.Flags().String("name", "", "Optional unique profile name")
	profilesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	profilesDownloadCmd.Flags().String("to", "", "Output zip file path")
	profilesDownloadCmd.Flags().Bool("pretty", false, "Pretty-print JSON to file")
}

func runProfilesList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.List(cmd.Context())
}

func runProfilesGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Get(cmd.Context(), ProfilesGetInput{Identifier: args[0]})
}

func runProfilesCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	name, _ := cmd.Flags().GetString("name")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Create(cmd.Context(), ProfilesCreateInput{Name: name})
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
	out, _ := cmd.Flags().GetString("to")
	pretty, _ := cmd.Flags().GetBool("pretty")
	svc := client.Profiles
	p := ProfilesCmd{profiles: &svc}
	return p.Download(cmd.Context(), ProfilesDownloadInput{Identifier: args[0], Output: out, Pretty: pretty})
}
