package proxies

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func (p ProxyCmd) Delete(ctx context.Context, in ProxyDeleteInput) error {
	if !in.SkipConfirm {
		// Try to get the proxy details for better confirmation message
		proxy, err := p.proxies.Get(ctx, in.ID)
		if err != nil {
			// If we can't get the proxy, just use the ID
			if !util.IsNotFound(err) {
				return util.CleanedUpSdkError{Err: err}
			}
			proxy = nil
		}

		var confirmMsg string
		if proxy != nil && proxy.Name != "" {
			confirmMsg = fmt.Sprintf("Are you sure you want to delete proxy '%s' (ID: %s)?", proxy.Name, in.ID)
		} else {
			confirmMsg = fmt.Sprintf("Are you sure you want to delete proxy '%s'?", in.ID)
		}

		pterm.DefaultInteractiveConfirm.DefaultText = confirmMsg
		result, _ := pterm.DefaultInteractiveConfirm.Show()
		if !result {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	pterm.Info.Printf("Deleting proxy: %s\n", in.ID)

	err := p.proxies.Delete(ctx, in.ID)
	if err != nil {
		if util.IsNotFound(err) {
			pterm.Warning.Printf("Proxy '%s' not found\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Successfully deleted proxy: %s\n", in.ID)
	return nil
}

func runProxiesDelete(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Delete(cmd.Context(), ProxyDeleteInput{
		ID:          args[0],
		SkipConfirm: skipConfirm,
	})
}
