package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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

		client := NewJobClient(url, token)
		result, err := client.RunJob(jobID)
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
	rootCmd.AddCommand(runCmd)
}
