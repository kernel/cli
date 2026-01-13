package cmd

import (
	"fmt"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:     "logs <app_name>",
	Aliases: []string{"log"},
	Short:   "Show logs for a Kernel application",
	Args:    cobra.ExactArgs(1),
	RunE:    runLogs,
}

func init() {
	logsCmd.Flags().String("version", "latest", "Specify a version of the app (default: latest)")
	logsCmd.Flags().BoolP("follow", "f", false, "Follow logs in real-time (stream continuously)")
	logsCmd.Flags().String("since", "s", "How far back to retrieve logs. Supports duration formats: ns, us, ms, s, m, h (e.g., 5m, 2h, 1h30m). Note: 'd' for days is NOT supported - use hours instead. Can also specify timestamps: 2006-01-02 (day), 2006-01-02T15:04 (minute), 2006-01-02T15:04:05 (second), 2006-01-02T15:04:05.000 (ms). Maximum lookback is 167h (just under 7 days). Defaults to 5m if not following, 5s if following.")
	logsCmd.Flags().Bool("with-timestamps", false, "Include timestamps in each log line")
	logsCmd.Flags().StringP("invocation", "i", "", "Show logs for a specific invocation/run of the app. Accepts full ID or unambiguous prefix. If the invocation is still running, streaming respects --follow.")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	appName := args[0]
	version, _ := cmd.Flags().GetString("version")
	follow, _ := cmd.Flags().GetBool("follow")
	since, _ := cmd.Flags().GetString("since")
	timestamps, _ := cmd.Flags().GetBool("with-timestamps")
	invocationRef, _ := cmd.Flags().GetString("invocation")
	if version == "" {
		version = "latest"
	}
	if !cmd.Flags().Changed("since") {
		if follow {
			since = "5s"
		} else {
			since = "5m"
		}
	}

	// If an invocation is specified, stream invocation-specific logs and return
	if invocationRef != "" {
		inv, err := client.Invocations.Get(cmd.Context(), invocationRef)
		if err != nil {
			return fmt.Errorf("failed to get invocation: %w", err)
		}
		if inv.AppName != appName {
			return fmt.Errorf("invocation %s does not belong to app \"%s\" (found app: %s)", inv.ID, appName, inv.AppName)
		}

		pterm.Info.Printf("Streaming logs for invocation \"%s\" of app \"%s\" (action: %s, status: %s)...\n", inv.ID, inv.AppName, inv.ActionName, inv.Status)
		if follow {
			pterm.Info.Println("Press Ctrl+C to exit")
		} else {
			pterm.Info.Println("Showing recent logs (timeout after 3s with no events)")
		}

		stream := client.Invocations.FollowStreaming(cmd.Context(), inv.ID, kernel.InvocationFollowParams{}, option.WithMaxRetries(0))
		if stream.Err() != nil {
			return fmt.Errorf("failed to follow streaming: %w", stream.Err())
		}

		if follow {
			for stream.Next() {
				data := stream.Current()
				switch data.Event {
				case "log":
					logEntry := data.AsLog()
					if timestamps {
						fmt.Printf("%s %s\n", util.FormatLocal(logEntry.Timestamp), logEntry.Message)
					} else {
						fmt.Println(logEntry.Message)
					}
				case "error":
					errEv := data.AsError()
					pterm.Error.Printfln("%s: %s", errEv.Error.Code, errEv.Error.Message)
				}
			}
		} else {
			timeout := time.NewTimer(3 * time.Second)
			defer timeout.Stop()

			done := false
			for !done {
				nextCh := make(chan bool, 1)
				go func() {
					hasNext := stream.Next()
					nextCh <- hasNext
				}()

				select {
				case hasNext := <-nextCh:
					if !hasNext {
						done = true
					} else {
						data := stream.Current()
						switch data.Event {
						case "log":
							logEntry := data.AsLog()
							if timestamps {
								fmt.Printf("%s %s\n", util.FormatLocal(logEntry.Timestamp), logEntry.Message)
							} else {
								fmt.Println(logEntry.Message)
							}
						case "error":
							errEv := data.AsError()
							pterm.Error.Printfln("%s: %s", errEv.Error.Code, errEv.Error.Message)
						}
						timeout.Reset(3 * time.Second)
					}
				case <-timeout.C:
					done = true
					stream.Close()
					return nil
				}
			}
		}

		if stream.Err() != nil {
			return fmt.Errorf("failed to follow streaming: %w", stream.Err())
		}
		return nil
	}

	params := kernel.AppListParams{
		AppName: kernel.Opt(appName),
		Version: kernel.Opt(version),
	}
	apps, err := client.Apps.List(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}
	if apps == nil || len(apps.Items) == 0 {
		return fmt.Errorf("app \"%s\" not found", appName)
	}
	if len(apps.Items) > 1 {
		return fmt.Errorf("multiple apps found for \"%s\", please specify a version", appName)
	}
	app := apps.Items[0]

	pterm.Info.Printf("Streaming logs for app \"%s\" (version: %s, id: %s)...\n", appName, version, app.ID)
	if follow {
		pterm.Info.Println("Press Ctrl+C to exit")
	} else {
		pterm.Info.Println("Showing recent logs (timeout after 3s with no events)")
	}

	stream := client.Deployments.FollowStreaming(cmd.Context(), app.Deployment, kernel.DeploymentFollowParams{
		Since: kernel.Opt(since),
	}, option.WithMaxRetries(0))
	if stream.Err() != nil {
		return fmt.Errorf("failed to follow streaming: %w", stream.Err())
	}

	// Handle follow vs non-follow mode
	if follow {
		// Keep streaming indefinitely
		for stream.Next() {
			data := stream.Current()
			switch data.Event {
			case "log":
				logEntry := data.AsLog()
				if timestamps {
					fmt.Printf("%s %s\n", util.FormatLocal(logEntry.Timestamp), logEntry.Message)
				} else {
					fmt.Println(logEntry.Message)
				}
			case "error":
				errEv := data.AsErrorEvent()
				pterm.Error.Printfln("%s: %s", errEv.Error.Code, errEv.Error.Message)
			}
		}
	} else {
		// Exit after 3 seconds of no activity
		timeout := time.NewTimer(3 * time.Second)
		defer timeout.Stop()

		done := false
		for !done {
			// Create a channel for the Next() operation
			nextCh := make(chan bool, 1)

			// Start a goroutine to check for the next event
			go func() {
				hasNext := stream.Next()
				nextCh <- hasNext
			}()

			// Wait for either next event or timeout
			select {
			case hasNext := <-nextCh:
				if !hasNext {
					done = true
				} else {
					// Got an event, display it and reset timer
					data := stream.Current()
					switch data.Event {
					case "log":
						logEntry := data.AsLog()
						if timestamps {
							fmt.Printf("%s %s\n", util.FormatLocal(logEntry.Timestamp), logEntry.Message)
						} else {
							fmt.Println(logEntry.Message)
						}
					case "error":
						errEv := data.AsErrorEvent()
						pterm.Error.Printfln("%s: %s", errEv.Error.Code, errEv.Error.Message)
					}
					timeout.Reset(3 * time.Second)
				}
			case <-timeout.C:
				// No events for 3 seconds, we're done
				done = true
				stream.Close()
				return nil
			}
		}
	}

	if stream.Err() != nil {
		return fmt.Errorf("failed to follow streaming: %w", stream.Err())
	}
	return nil
}
