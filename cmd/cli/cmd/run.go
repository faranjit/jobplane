package cmd

import (
	"jobplane/pkg/api"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var scheduleAt string

var runCmd = &cobra.Command{
	Use:   "run [job_id]",
	Short: "Trigger a new execution for a job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jobID := args[0]

		url := viper.GetString("url")
		token := viper.GetString("token")

		if token == "" {
			cmd.Println("API token not found. Please set it using the --token flag or the JOBPLANE_TOKEN environment variable")
			return
		}

		var req api.RunJobRequest
		if scheduleAt != "" {
			t, err := time.Parse(time.RFC3339, scheduleAt)
			if err != nil {
				cmd.Printf("Invalid schedule format. Use RFC3339 (e.g. 2024-01-01T12:00:00Z): %v\n", err)
				return
			}

			req.ScheduledAt = &t
		}

		client := NewJobClient(url, token)
		result, err := client.RunJob(jobID, req)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				cmd.Printf("Error (%d): %s\n", apiErr.StatusCode, apiErr.Message)
			} else {
				cmd.Printf("Error: %v\n", err)
			}
			return
		}

		cmd.Printf("🚀 Execution started!\nID: %s\n", result.ExecutionID)
	},
}

func init() {
	runCmd.Flags().StringVar(&scheduleAt, "schedule", "", "Schedule execution at specific time (RFC3339)")
	rootCmd.AddCommand(runCmd)
}
