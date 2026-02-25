package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type statusComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type statusGroup struct {
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Components []statusComponent `json:"components"`
}

type statusResponse struct {
	Status string        `json:"status"`
	Groups []statusGroup `json:"groups"`
}

const defaultBaseURL = "https://api.onkernel.com"

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the operational status of Kernel services",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringP("output", "o", "", "Output format (json)")
}

func getBaseURL() string {
	if u := os.Getenv("KERNEL_BASE_URL"); strings.TrimSpace(u) != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBaseURL
}

func runStatus(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(getBaseURL() + "/status")
	if err != nil {
		pterm.Error.Println("Could not reach Kernel API. Check https://status.kernel.sh for updates.")
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		pterm.Error.Println("Could not reach Kernel API. Check https://status.kernel.sh for updates.")
		return fmt.Errorf("status request failed: %s", resp.Status)
	}

	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	printStatus(status)
	return nil
}

// Colors match the dashboard's api-status-indicator.tsx
var statusDisplay = map[string]struct {
	label string
	rgb   pterm.RGB
}{
	"operational":          {label: "Operational", rgb: pterm.NewRGB(31, 163, 130)},
	"degraded_performance": {label: "Degraded Performance", rgb: pterm.NewRGB(245, 158, 11)},
	"partial_outage":       {label: "Partial Outage", rgb: pterm.NewRGB(242, 85, 51)},
	"full_outage":          {label: "Major Outage", rgb: pterm.NewRGB(239, 68, 68)},
	"maintenance":          {label: "Maintenance", rgb: pterm.NewRGB(36, 99, 235)},
	"unknown":              {label: "Unknown", rgb: pterm.NewRGB(128, 128, 128)},
}

func getStatusDisplay(status string) (string, pterm.RGB) {
	if d, ok := statusDisplay[status]; ok {
		return d.label, d.rgb
	}
	return "Unknown", pterm.NewRGB(128, 128, 128)
}

func coloredDot(rgb pterm.RGB) string {
	return rgb.Sprint("●")
}

func printStatus(resp statusResponse) {
	label, rgb := getStatusDisplay(resp.Status)
	header := fmt.Sprintf("Kernel Status: %s", rgb.Sprint(label))
	pterm.Println()
	pterm.Println("  " + header)

	for _, group := range resp.Groups {
		pterm.Println()
		if len(group.Components) == 0 {
			groupLabel, groupColor := getStatusDisplay(group.Status)
			pterm.Printf("  %s %s  %s\n", coloredDot(groupColor), pterm.Bold.Sprint(group.Name), groupLabel)
		} else {
			pterm.Println("  " + pterm.Bold.Sprint(group.Name))
			for _, comp := range group.Components {
				compLabel, compColor := getStatusDisplay(comp.Status)
				pterm.Printf("    %s %-20s %s\n", coloredDot(compColor), comp.Name, compLabel)
			}
		}
	}
	pterm.Println()
}
