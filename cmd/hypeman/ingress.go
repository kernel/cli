package hypeman

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var ingressCmd = &cobra.Command{
	Use:     "ingress",
	Aliases: []string{"ingresses"},
	Short:   "Manage Hypeman ingresses",
}

var ingressCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an ingress",
	RunE:  runIngressCreate,
}

var ingressListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ingresses",
	RunE:  runIngressList,
}

var ingressGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get ingress details",
	Args:  cobra.ExactArgs(1),
	RunE:  runIngressGet,
}

var ingressDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an ingress",
	Args:  cobra.ExactArgs(1),
	RunE:  runIngressDelete,
}

func init() {
	ingressCmd.AddCommand(ingressCreateCmd)
	ingressCmd.AddCommand(ingressListCmd)
	ingressCmd.AddCommand(ingressGetCmd)
	ingressCmd.AddCommand(ingressDeleteCmd)

	// ingress create flags
	ingressCreateCmd.Flags().String("name", "", "Human-readable name (required)")
	_ = ingressCreateCmd.MarkFlagRequired("name")
	ingressCreateCmd.Flags().StringArray("rule", nil, "Routing rule as JSON (repeatable, required)")
	_ = ingressCreateCmd.MarkFlagRequired("rule")
	ingressCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	// Simple rule flags for single-rule ingresses
	ingressCreateCmd.Flags().String("hostname", "", "Match hostname (shorthand for single rule)")
	ingressCreateCmd.Flags().Int64("match-port", 0, "Match port (shorthand for single rule)")
	ingressCreateCmd.Flags().String("target-instance", "", "Target instance name or ID (shorthand for single rule)")
	ingressCreateCmd.Flags().Int64("target-port", 0, "Target port (shorthand for single rule)")
	ingressCreateCmd.Flags().Bool("tls", false, "Enable TLS termination (shorthand for single rule)")
	ingressCreateCmd.Flags().Bool("redirect-http", false, "Redirect HTTP to HTTPS (shorthand for single rule)")

	// ingress list flags
	ingressListCmd.Flags().StringP("output", "o", "", "Output format: json")

	// ingress get flags
	ingressGetCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runIngressCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")

	// Build rules from --rule JSON flags
	ruleStrs, _ := cmd.Flags().GetStringArray("rule")
	var rules []hypemansdk.IngressRuleParam
	for _, ruleStr := range ruleStrs {
		var rule hypemansdk.IngressRuleParam
		if err := json.Unmarshal([]byte(ruleStr), &rule); err != nil {
			return fmt.Errorf("invalid rule JSON: %w", err)
		}
		rules = append(rules, rule)
	}

	// If no --rule flags but shorthand flags are provided, build a single rule
	if len(rules) == 0 {
		hostname, _ := cmd.Flags().GetString("hostname")
		targetInstance, _ := cmd.Flags().GetString("target-instance")
		targetPort, _ := cmd.Flags().GetInt64("target-port")

		if hostname != "" && targetInstance != "" && targetPort > 0 {
			rule := hypemansdk.IngressRuleParam{
				Match: hypemansdk.IngressMatchParam{
					Hostname: hostname,
				},
				Target: hypemansdk.IngressTargetParam{
					Instance: targetInstance,
					Port:     targetPort,
				},
			}
			if matchPort, _ := cmd.Flags().GetInt64("match-port"); matchPort > 0 {
				rule.Match.Port = param.NewOpt(matchPort)
			}
			if cmd.Flags().Changed("tls") {
				tls, _ := cmd.Flags().GetBool("tls")
				rule.Tls = param.NewOpt(tls)
			}
			if cmd.Flags().Changed("redirect-http") {
				redir, _ := cmd.Flags().GetBool("redirect-http")
				rule.RedirectHTTP = param.NewOpt(redir)
			}
			rules = append(rules, rule)
		}
	}

	if len(rules) == 0 {
		return fmt.Errorf("at least one --rule is required")
	}

	ingress, err := client.Ingresses.New(cmd.Context(), hypemansdk.IngressNewParams{
		Name:  name,
		Rules: rules,
	})
	if err != nil {
		return fmt.Errorf("failed to create ingress: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(ingress)
	}

	pterm.Success.Printf("Created ingress %s (%s)\n", ingress.Name, ingress.ID)
	printIngressDetail(ingress)
	return nil
}

func runIngressList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	ingresses, err := client.Ingresses.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list ingresses: %w", err)
	}

	if output == "json" {
		if ingresses == nil || len(*ingresses) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*ingresses)
	}

	if ingresses == nil || len(*ingresses) == 0 {
		pterm.Info.Println("No ingresses found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Name", "Rules", "Created At"}}
	for _, ing := range *ingresses {
		var ruleDescs []string
		for _, r := range ing.Rules {
			ruleDescs = append(ruleDescs, fmt.Sprintf("%s -> %s:%d", r.Match.Hostname, r.Target.Instance, r.Target.Port))
		}
		tableData = append(tableData, []string{
			ing.ID,
			ing.Name,
			strings.Join(ruleDescs, "; "),
			util.FormatLocal(ing.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runIngressGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	ingress, err := client.Ingresses.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get ingress: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(ingress)
	}

	printIngressDetail(ingress)
	return nil
}

func runIngressDelete(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Ingresses.Delete(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to delete ingress: %w", err)
	}

	pterm.Success.Printf("Deleted ingress %s\n", args[0])
	return nil
}

func printIngressDetail(ing *hypemansdk.Ingress) {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", ing.ID},
		{"Name", ing.Name},
		{"Created At", util.FormatLocal(ing.CreatedAt)},
	}
	for i, r := range ing.Rules {
		prefix := fmt.Sprintf("Rule %d", i+1)
		tableData = append(tableData,
			[]string{prefix + " Hostname", r.Match.Hostname},
			[]string{prefix + " Match Port", fmt.Sprintf("%d", r.Match.Port)},
			[]string{prefix + " Target", fmt.Sprintf("%s:%d", r.Target.Instance, r.Target.Port)},
			[]string{prefix + " TLS", fmt.Sprintf("%t", r.Tls)},
			[]string{prefix + " Redirect HTTP", fmt.Sprintf("%t", r.RedirectHTTP)},
		)
	}
	table.PrintTableNoPad(tableData, true)
}
