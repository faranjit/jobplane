package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jobplane/pkg/api"

	"github.com/spf13/viper"
)

func TestStatusCommand_Success(t *testing.T) {
	resetViper()

	startTime := time.Now().Add(-10 * time.Minute)
	endTime := time.Now().Add(-9 * time.Minute)
	exitCode := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/executions/exec-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got: %s", r.Header.Get("Authorization"))
		}

		resp := api.ExecutionResponse{
			ID:          "exec-123",
			Status:      "SUCCEEDED",
			Attempt:     1,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
			ExitCode:    &exitCode,
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
	rootCmd.SetArgs([]string{"status", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "exec-123") {
		t.Errorf("expected execution ID in output, got: %s", output)
	}
	if !strings.Contains(output, "SUCCEEDED") {
		t.Errorf("expected SUCCEEDED status, got: %s", output)
	}
	if !strings.Contains(output, "Attempt") {
		t.Errorf("expected Attempt field, got: %s", output)
	}
	if strings.Contains(output, "Result:") {
		t.Errorf("expected no Result line when result is nil, got: %s", output)
	}
}

func TestStatusCommand_WithResult(t *testing.T) {
	resetViper()

	startTime := time.Now().Add(-10 * time.Minute)
	endTime := time.Now().Add(-9 * time.Minute)
	exitCode := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.ExecutionResponse{
			ID:          "exec-with-result",
			Status:      "SUCCEEDED",
			Attempt:     1,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
			ExitCode:    &exitCode,
			Result:      json.RawMessage(`{"score":42,"status":"ok"}`),
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
	rootCmd.SetArgs([]string{"status", "exec-with-result"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Result:") {
		t.Errorf("expected Result line in output, got: %s", output)
	}
	if !strings.Contains(output, "score") {
		t.Errorf("expected 'score' in result, got: %s", output)
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected '42' in result, got: %s", output)
	}
}

func TestStatusCommand_MissingToken(t *testing.T) {
	resetViper()

	viper.Set("url", "http://localhost:6161")
	viper.Set("token", "")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"status", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "API token not found") {
		t.Errorf("expected token error message, got: %s", output)
	}
}

func TestStatusCommand_NotFound(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"status", "non-existent"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Request failed with status code: 404") {
		t.Errorf("expected 404 error, got: %s", output)
	}
}

func TestStatusCommand_ServerError(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"status", "exec-123"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Request failed with status code: 500") {
		t.Errorf("expected 500 error, got: %s", output)
	}
}

func TestStatusCommand_RequiresExecutionIDArgument(t *testing.T) {
	resetViper()
	viper.Set("token", "test-token")

	var stderr bytes.Buffer
	rootCmd.SetOut(&stderr)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"status"}) // No execution ID

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when no execution ID provided")
	}
}

func TestStatusCommand_FailedExecution(t *testing.T) {
	resetViper()

	startTime := time.Now().Add(-5 * time.Minute)
	endTime := time.Now().Add(-4 * time.Minute)
	exitCode := 1
	errMsg := "Container crashed"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.ExecutionResponse{
			ID:          "exec-456",
			Status:      "FAILED",
			Attempt:     3,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
			ExitCode:    &exitCode,
			Error:       &errMsg,
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
	rootCmd.SetArgs([]string{"status", "exec-456"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "FAILED") {
		t.Errorf("expected FAILED status, got: %s", output)
	}
	if !strings.Contains(output, "Container crashed") {
		t.Errorf("expected error message, got: %s", output)
	}
}

func TestStatusCommand_RunningExecution(t *testing.T) {
	resetViper()

	startTime := time.Now().Add(-1 * time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.ExecutionResponse{
			ID:        "exec-789",
			Status:    "RUNNING",
			Attempt:   1,
			StartedAt: &startTime,
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
	rootCmd.SetArgs([]string{"status", "exec-789"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "RUNNING") {
		t.Errorf("expected RUNNING status, got: %s", output)
	}
}

func TestStatusCommand_PendingExecution(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.ExecutionResponse{
			ID:      "exec-pending",
			Status:  "PENDING",
			Attempt: 0,
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
	rootCmd.SetArgs([]string{"status", "exec-pending"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PENDING") {
		t.Errorf("expected PENDING status, got: %s", output)
	}
}

func TestColorizeStatus(t *testing.T) {
	tests := []struct {
		status   string
		contains string
	}{
		{"SUCCEEDED", "SUCCEEDED"},
		{"FAILED", "FAILED"},
		{"RUNNING", "RUNNING"},
		{"PENDING", "PENDING"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		result := colorizeStatus(tt.status)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("colorizeStatus(%s) should contain %s, got: %s", tt.status, tt.contains, result)
		}
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status   string
		contains string
	}{
		{"SUCCEEDED", "✓"},
		{"FAILED", "✗"},
		{"RUNNING", "⏳"},
		{"PENDING", "◯"},
		{"UNKNOWN", "•"},
	}

	for _, tt := range tests {
		result := statusIcon(tt.status)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("statusIcon(%s) should contain %s, got: %s", tt.status, tt.contains, result)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{65 * time.Second, "1m 5s"},
		{125 * time.Minute, "2h 5m"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.duration, result, tt.expected)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		offset   time.Duration
		contains string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{48 * time.Hour, "2 days"},
	}

	for _, tt := range tests {
		testTime := time.Now().Add(-tt.offset)
		result := relativeTime(testTime)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("relativeTime(%v ago) should contain %s, got: %s", tt.offset, tt.contains, result)
		}
	}
}
