package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var dlqCmd = &cobra.Command{
	Use:   "dlq",
	Short: "Manage the Dead Letter Queue (DLQ)",
	Long:  `Inspect and retry executions that have permanently failed after exceeding their retry limit.`,
}

var dlqListCmd = &cobra.Command{
	Use:   "list",
	Short: "List failed executions in the DLQ",
	Run: func(cmd *cobra.Command, args []string) {
		client := NewJobClient(viper.GetString("url"), viper.GetString("token"))

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		// Fetch DLQ items with pagination
		executions, err := client.ListDLQExecutions(limit, offset)
		if err != nil {
			cmd.Printf("Error fetching DLQ: %s\n", err)
			os.Exit(1)
		}

		if len(executions) == 0 {
			if offset > 0 {
				cmd.Println("No more executions found in DLQ.")
			} else {
				cmd.Println("No executions found in DLQ.")
			}
			return
		}

		// Print table
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "EXECUTION ID\tJOB\tATTEMPTS\tFAILED AT\tERROR")
		for _, e := range executions {
			failedAt := ""
			if e.FailedAt != nil {
				failedAt = e.FailedAt.Format(time.RFC3339)
			}
			errMsg := ""
			if e.ErrorMessage != nil {
				// Truncate long error messages for the table view
				errMsg = *e.ErrorMessage
				if len(errMsg) > 50 {
					errMsg = errMsg[:47] + "..."
				}
			}

			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
				e.ExecutionID,
				e.JobName,
				e.Attempts,
				failedAt,
				errMsg,
			)
		}
		w.Flush()
	},
}

var dlqRetryCmd = &cobra.Command{
	Use:   "retry [execution_id]",
	Short: "Retry a specific execution from the DLQ",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]
		client := NewJobClient(viper.GetString("url"), viper.GetString("token"))

		resp, err := client.RetryDLQExecution(executionID)
		if err != nil {
			cmd.Printf("Error retrying execution: %s\n", err)
			os.Exit(1)
		}

		cmd.Printf("✅ Execution %s retried successfully.\n", executionID)
		cmd.Printf("   New Execution ID: %s\n", resp.NewExecutionID)
	},
}

func init() {
	rootCmd.AddCommand(dlqCmd)
	dlqCmd.AddCommand(dlqListCmd)
	dlqCmd.AddCommand(dlqRetryCmd)

	dlqListCmd.Flags().IntP("limit", "l", 20, "Number of items demanded in the DQL list")
	dlqListCmd.Flags().IntP("offset", "o", 0, "Offset for pagination")
}
