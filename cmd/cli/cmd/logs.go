package cmd

import (
	"encoding/json"
	"fmt"
	"jobplane/pkg/api"
	"net/http"
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

		var lastID int64 = 0
		client := &http.Client{Timeout: 30 * time.Second}

		for {
			newLogs, err := fetchLogs(client, url, token, executionID, lastID)
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

func fetchLogs(client *http.Client, baseURL, token, execID string, afterID int64) ([]api.LogEntry, error) {
	endpoint := fmt.Sprintf("%s/executions/%s/logs?after_id=%d", baseURL, execID, afterID)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result api.GetLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Logs, nil
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
}
