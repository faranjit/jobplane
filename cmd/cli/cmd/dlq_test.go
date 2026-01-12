package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestDLQList_Success(t *testing.T) {
	resetViper()

	// Mock server returning a list of failed executions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/executions/dlq") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		failedAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		errMsg := "runtime error: out of memory"

		resp := []api.DLQExecutionResponse{
			{
				ID:           1,
				ExecutionID:  "exec-dead-1",
				JobID:        "job-1",
				JobName:      "memory-hog",
				Priority:     50,
				ErrorMessage: &errMsg,
				Attempts:     6,
				FailedAt:     &failedAt,
			},
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"dlq", "list"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()

	// Verify table headers and content presence
	expectedStrings := []string{
		"EXECUTION ID", "JOB", "ATTEMPTS", "ERROR", // Headers
		"exec-dead-1", "memory-hog", "runtime error: out of memory", // Data
	}

	for _, s := range expectedStrings {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q, got:\n%s", s, output)
		}
	}
}

func TestDLQList_Pagination(t *testing.T) {
	resetViper()

	// Mock server verifying query parameters
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("limit") != "5" {
			t.Errorf("expected limit=5, got %s", query.Get("limit"))
		}
		if query.Get("offset") != "10" {
			t.Errorf("expected offset=10, got %s", query.Get("offset"))
		}

		// Return empty list to keep test simple
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]api.DLQExecutionResponse{})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	// Pass pagination flags
	rootCmd.SetArgs([]string{"dlq", "list", "--limit", "5", "--offset", "10"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDLQList_Empty(t *testing.T) {
	resetViper()
	dlqListCmd.Flags().Set("limit", "20")
	dlqListCmd.Flags().Set("offset", "0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]api.DLQExecutionResponse{})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"dlq", "list"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No executions found in DLQ.") {
		t.Errorf("expected empty message, got: %s", output)
	}
}

func TestDLQRetry_Success(t *testing.T) {
	resetViper()

	targetID := "exec-dead-1"
	newID := "exec-retry-2"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		expectedPath := fmt.Sprintf("/executions/dlq/%s/retry", targetID)
		if !strings.HasSuffix(r.URL.Path, expectedPath) {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(api.RetryDQLExecutionResponse{
			NewExecutionID: newID,
		})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"dlq", "retry", targetID})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "retried successfully") {
		t.Errorf("expected success message, got: %s", output)
	}
	if !strings.Contains(output, newID) {
		t.Errorf("expected new execution ID %s in output, got: %s", newID, output)
	}
}

func TestDLQRetry_MissingArg(t *testing.T) {
	resetViper()
	viper.Set("token", "test-token")

	var stderr bytes.Buffer
	rootCmd.SetOut(&stderr)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"dlq", "retry"}) // Missing ID

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when missing execution ID argument")
	}

	// Cobra usually prints usage or error message to stderr
	output := stderr.String()
	if !strings.Contains(output, "accepts 1 arg(s)") && !strings.Contains(output, "requires") {
		// Exact message depends on cobra version, but should indicate missing arg
		// We just ensure it failed.
	}
}
