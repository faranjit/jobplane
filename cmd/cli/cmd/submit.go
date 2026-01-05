package cmd

import (
	"jobplane/pkg/api"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Create and immediately run a job",
	Long: `Create a new job definition and immediately trigger an execution.

This is a convenience command that combines 'create' and 'run' into a single step.

Example:
  jobctl submit --name "my-job" --image "alpine:latest" --command "echo,hello"
  jobctl submit --name "python-script" --image "python:3.11" --command "python,-c,print('hello')" --timeout 300`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		name, _ := flags.GetString("name")
		image, _ := flags.GetString("image")
		command, _ := flags.GetStringSlice("command")
		timeout, _ := flags.GetInt("timeout")

		url := viper.GetString("url")
		token := viper.GetString("token")

		if token == "" {
			cmd.Println("API token not found. Please set it using the --token flag or the JOBPLANE_TOKEN environment variable")
			return
		}

		if name == "" {
			cmd.Println("Error: --name is required")
			return
		}

		if image == "" {
			cmd.Println("Error: --image is required")
			return
		}

		if len(command) == 0 {
			cmd.Println("Error: --command is required")
			return
		}

		client := NewJobClient(url, token)

		// Step 1: Create the job
		createReq := api.CreateJobRequest{
			Name:           name,
			Image:          image,
			Command:        command,
			DefaultTimeout: timeout,
		}

		createResult, err := client.CreateJob(createReq)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				cmd.Printf("Create failed (%d): %s\n", apiErr.StatusCode, apiErr.Message)
			} else {
				cmd.Printf("Create failed: %v\n", err)
			}
			return
		}

		// Step 2: Run the job
		// Empty request since submit triggers execution immediately
		runResult, err := client.RunJob(createResult.JobID, api.RunJobRequest{})
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				cmd.Printf("Job created (ID: %s) but run failed (%d): %s\n", createResult.JobID, apiErr.StatusCode, apiErr.Message)
			} else {
				cmd.Printf("Job created (ID: %s) but run failed: %v\n", createResult.JobID, err)
			}
			return
		}

		cmd.Printf("✓ Job submitted!\nJob ID: %s\nExecution ID: %s\n", createResult.JobID, runResult.ExecutionID)
	},
}

func init() {
	flags := submitCmd.Flags()
	flags.StringP("name", "n", "", "Name of the job (required)")
	flags.StringP("image", "i", "", "Container image or 'ignored' for exec runtime (required)")
	flags.StringSliceP("command", "c", []string{}, "Command to execute (required)")
	flags.Int("timeout", 0, "Default timeout in seconds (optional)")

	rootCmd.AddCommand(submitCmd)
}
