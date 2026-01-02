package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

		endpoint := fmt.Sprintf("%s/jobs/%s/run", url, jobID)
		req, err := http.NewRequest(http.MethodPost, endpoint, nil)
		if err != nil {
			cmd.Printf("Failed to create request: %v\n", err)
			return
		}

		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			cmd.Printf("Request failed: %v\n", err)
			return
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			cmd.Printf("Error (%d): %s\n", resp.StatusCode, string(bodyBytes))
			return
		}

		var result map[string]string
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			cmd.Println("Execution started (failed to parse response)")
			return
		}

		cmd.Printf("🚀 Execution started!\nID: %s\n", result["execution_id"])
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
