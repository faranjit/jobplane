package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"jobplane/pkg/api"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var statusCmd = &cobra.Command{
	Use:   "status [execution_id]",
	Short: "Get status of an execution",
	Long:  `Retrieve detailed status information for a job execution, including its current state (PENDING, RUNNING, SUCCEEDED, FAILED), exit code, and timestamps.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]

		url := viper.GetString("url")
		token := viper.GetString("token")

		if token == "" {
			cmd.Println("API token not found. Please set it using the --token flag or the JOBPLANE_TOKEN environment variable")
			return
		}

		endpoint := fmt.Sprintf("%s/executions/%s", url, executionID)
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			cmd.Printf("Failed to create request: %v\n", err)
			return
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			cmd.Printf("Failed to send request: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			cmd.Printf("Request failed with status code: %d\n", resp.StatusCode)
			return
		}

		body, _ := io.ReadAll(resp.Body)

		var execution api.ExecutionResponse
		if err := json.Unmarshal(body, &execution); err != nil {
			cmd.Printf("Failed to parse response: %v\n", err)
			return
		}

		printStatus(cmd, execution)
	},
}

func printStatus(cmd *cobra.Command, execution api.ExecutionResponse) {
	// Header with status icon
	icon := statusIcon(execution.Status)
	cmd.Printf("%s %sExecution Details%s\n", icon, colorBold, colorReset)
	cmd.Println("──────────────────────────────")

	// ID
	cmd.Printf("%sID:%s          %s\n", colorDim, colorReset, execution.ID)

	// Status with icon
	cmd.Printf("%sStatus:%s      %s\n", colorDim, colorReset, colorizeStatus(execution.Status))

	// Attempt
	cmd.Printf("%sAttempt:%s     %d\n", colorDim, colorReset, execution.Attempt)

	// Exit Code
	if execution.ExitCode != nil {
		exitCode := *execution.ExitCode
		if exitCode == 0 {
			cmd.Printf("%sExit Code:%s   %s%d%s\n", colorDim, colorReset, colorGreen, exitCode, colorReset)
		} else {
			cmd.Printf("%sExit Code:%s   %s%d%s\n", colorDim, colorReset, colorRed, exitCode, colorReset)
		}
	} else {
		cmd.Printf("%sExit Code:%s   -\n", colorDim, colorReset)
	}

	// Error (if present)
	if execution.Error != nil {
		cmd.Printf("%sError:%s       %s%s%s\n", colorDim, colorReset, colorRed, *execution.Error, colorReset)
	}

	// Timestamps with relative time
	cmd.Printf("%sStarted:%s     %s\n", colorDim, colorReset, formatTimeWithRelative(execution.StartedAt))

	// Duration if both times available
	if execution.StartedAt != nil && execution.CompletedAt != nil {
		duration := execution.CompletedAt.Sub(*execution.StartedAt)
		cmd.Printf("%sFinished:%s    %s %s(%s)%s\n", colorDim, colorReset,
			formatTimeWithRelative(execution.CompletedAt),
			colorCyan, formatDuration(duration), colorReset)
	} else {
		cmd.Printf("%sFinished:%s    %s\n", colorDim, colorReset, formatTimeWithRelative(execution.CompletedAt))
	}
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func statusIcon(status string) string {
	switch status {
	case "SUCCEEDED":
		return colorGreen + "✓" + colorReset
	case "FAILED":
		return colorRed + "✗" + colorReset
	case "RUNNING":
		return colorYellow + "⏳" + colorReset
	case "PENDING":
		return colorCyan + "◯" + colorReset
	default:
		return "•"
	}
}

func colorizeStatus(status string) string {
	icon := statusIcon(status)
	switch status {
	case "SUCCEEDED":
		return icon + " " + colorGreen + status + colorReset
	case "FAILED":
		return icon + " " + colorRed + status + colorReset
	case "RUNNING":
		return icon + " " + colorYellow + status + colorReset
	case "PENDING":
		return icon + " " + colorCyan + status + colorReset
	default:
		return status
	}
}

func formatTimeWithRelative(t *time.Time) string {
	if t == nil {
		return "-"
	}
	relative := relativeTime(*t)
	return fmt.Sprintf("%s %s(%s ago)%s", t.Format("Mon, 02 Jan 2006 15:04:05 MST"), colorDim, relative, colorReset)
}

func relativeTime(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
