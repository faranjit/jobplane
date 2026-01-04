package cmd

import (
	"bytes"
	"encoding/json"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLogsCommand_Success(t *testing.T) {
	resetViper()

	callCount := 0
	// Mock server that returns logs on first call, empty on second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/executions/exec-123/logs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got: %s", r.Header.Get("Authorization"))
		}

		var resp api.GetLogsResponse
		if callCount == 1 {
			resp = api.GetLogsResponse{
				Logs: []api.LogEntry{
					{ID: 1, Content: "Log line 1\n"},
					{ID: 2, Content: "Log line 2\n"},
				},
			}
		} else {
			// Return empty to terminate loop
			resp = api.GetLogsResponse{Logs: []api.LogEntry{}}
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
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Log line 1") {
		t.Errorf("expected log line 1, got: %s", output)
	}
	if !strings.Contains(output, "Log line 2") {
		t.Errorf("expected log line 2, got: %s", output)
	}
}

func TestLogsCommand_MissingToken(t *testing.T) {
	resetViper()

	viper.Set("url", "http://localhost:6161")
	viper.Set("token", "")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "API token not found") {
		t.Errorf("expected token error message, got: %s", output)
	}
}

func TestLogsCommand_ServerError(t *testing.T) {
	resetViper()

	// Mock server returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error fetching logs") {
		t.Errorf("expected error message, got: %s", output)
	}
}

func TestLogsCommand_NotFoundError(t *testing.T) {
	resetViper()

	// Mock server returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"logs", "non-existent"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error fetching logs") {
		t.Errorf("expected error message, got: %s", output)
	}
}

func TestLogsCommand_RequiresExecutionIDArgument(t *testing.T) {
	resetViper()
	viper.Set("token", "test-token")

	var stderr bytes.Buffer
	rootCmd.SetOut(&stderr)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"logs"}) // No execution ID

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when no execution ID provided")
	}
}

func TestLogsCommand_EmptyLogs(t *testing.T) {
	resetViper()

	// Mock server returns empty logs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.GetLogsResponse{
			Logs: []api.LogEntry{},
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
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete without error, output might be empty
	output := stdout.String()
	// Empty logs should not cause error, just no output
	if strings.Contains(output, "Error") {
		t.Errorf("unexpected error in output: %s", output)
	}
}

func TestLogsCommand_LogWithoutNewline(t *testing.T) {
	resetViper()

	callCount := 0
	// Mock server returns log without trailing newline on first call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp api.GetLogsResponse
		if callCount == 1 {
			resp = api.GetLogsResponse{
				Logs: []api.LogEntry{
					{ID: 1, Content: "Log without newline"},
				},
			}
		} else {
			resp = api.GetLogsResponse{Logs: []api.LogEntry{}}
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
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Log without newline") {
		t.Errorf("expected log content, got: %s", output)
	}
}

func TestLogsCommand_Pagination(t *testing.T) {
	resetViper()

	callCount := 0
	// Mock server returns logs in pages
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp api.GetLogsResponse

		if callCount == 1 {
			// First call returns first page
			resp = api.GetLogsResponse{
				Logs: []api.LogEntry{
					{ID: 1, Content: "Page 1 log\n"},
				},
			}
		} else {
			// Second call returns empty (end of logs)
			resp = api.GetLogsResponse{
				Logs: []api.LogEntry{},
			}
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
	rootCmd.SetArgs([]string{"logs", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Page 1 log") {
		t.Errorf("expected page 1 log, got: %s", output)
	}

	// Verify pagination happened (at least 2 calls)
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls for pagination, got %d", callCount)
	}
}

func TestLogsCommand_HasFollowFlag(t *testing.T) {
	// Check that logs command has the -f/--follow flag
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "logs [execution_id]" {
			flag := cmd.Flags().Lookup("follow")
			if flag != nil {
				found = true
				if flag.Shorthand != "f" {
					t.Errorf("expected shorthand 'f', got '%s'", flag.Shorthand)
				}
			}
			break
		}
	}

	if !found {
		t.Error("expected 'follow' flag on logs command")
	}
}

func TestFetchLogs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify after_id query param
		afterID := r.URL.Query().Get("after_id")
		if afterID != "10" {
			t.Errorf("expected after_id=10, got %s", afterID)
		}

		resp := api.GetLogsResponse{
			Logs: []api.LogEntry{
				{ID: 11, Content: "New log\n"},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewJobClient(server.URL, "test-token")
	logs, err := client.GetLogs("exec-123", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ID != 11 {
		t.Errorf("expected log ID 11, got %d", logs[0].ID)
	}
}

func TestFetchLogs_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewJobClient(server.URL, "test-token")
	_, err := client.GetLogs("exec-123", 0)
	if err == nil {
		t.Error("expected error for 403 status")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to contain 403, got: %v", err)
	}
}

func TestFetchLogs_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-valid-json"))
	}))
	defer server.Close()

	client := NewJobClient(server.URL, "test-token")
	_, err := client.GetLogs("exec-123", 0)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
