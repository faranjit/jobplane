package cmd

import (
	"jobplane/pkg/api"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new job definition",
	Long: `Create a new job definition (blueprint) that can be run later.

Example:
  jobctl create --name "my-job" --image "alpine:latest" --command "echo", "hello"
  jobctl create --name "python-script" --image "python:3.11" --command "python" "-c" "print('hello')" --timeout 300`,
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
		req := api.CreateJobRequest{
			Name:           name,
			Image:          image,
			Command:        command,
			DefaultTimeout: timeout,
		}

		result, err := client.CreateJob(req)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				cmd.Printf("Error (%d): %s\n", apiErr.StatusCode, apiErr.Message)
			} else {
				cmd.Printf("Error: %v\n", err)
			}
			return
		}

		cmd.Printf("✓ Job created!\nID: %s\nName: %s\n", result.JobID, name)
	},
}

func init() {
	flags := createCmd.Flags()
	flags.StringP("name", "n", "", "Name of the job (required)")
	flags.StringP("image", "i", "", "Container image or 'ignored' for exec runtime (required)")
	flags.StringSliceP("command", "c", []string{}, "Command to execute (required)")
	flags.Int("timeout", 0, "Default timeout in seconds (optional)")

	rootCmd.AddCommand(createCmd)
}
