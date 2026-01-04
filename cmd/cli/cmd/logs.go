package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var follow bool

var logsCmd = &cobra.Command{
	Use:   "logs [execution_id]",
	Short: "Stream logs for an execution",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]

		url := viper.GetString("url")
		token := viper.GetString("token")

		if token == "" {
			cmd.Println("API token not found. Please set it using the --token flag or the JOBPLANE_TOKEN environment variable")
			return
		}

		// Trap Ctrl+C to exit gracefully
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			os.Exit(0)
		}()

		client := NewJobClient(url, token)
		var lastID int64 = 0

		for {
			newLogs, err := client.GetLogs(executionID, lastID)
			if err != nil {
				cmd.Printf("Error fetching logs: %v\n", err)
				if !follow {
					break
				}
				time.Sleep(2 * time.Second) // Retry backoff
				continue
			}

			// Print new logs
			for _, log := range newLogs {
				cmd.Print(log.Content) // Content should contain newline if it came from runtime
				// If the log line doesn't end with newline, add it (optional, depends on raw data)
				if len(log.Content) > 0 && log.Content[len(log.Content)-1] != '\n' {
					cmd.Println()
				}

				if log.ID > lastID {
					lastID = log.ID
				}
			}

			// Logic for stopping or waiting
			if !follow {
				// If not following, and we got fewer logs than limit (assumed), we are done.
				// Or simply, if we got 0 logs, we assume we caught up.
				if len(newLogs) == 0 {
					break
				}
				// If we got logs, loop immediately to fetch next page
				continue
			}

			// If following, wait before polling again
			time.Sleep(1 * time.Second)
		}
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
}
