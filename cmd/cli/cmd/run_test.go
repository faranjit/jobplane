package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// resetViper clears viper config between tests for isolation
func resetViper() {
	viper.Reset()
	viper.SetEnvPrefix("JOBPLANE")
	viper.AutomaticEnv()
}

func TestRunCommand_Success(t *testing.T) {
	resetViper()

	// Setup mock server that returns a successful execution response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/jobs/test-job-id/run") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got: %s", r.Header.Get("Content-Type"))
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"execution_id": "exec-123",
		})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	// Capture output using root command
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Execution started") {
		t.Errorf("expected success message, got: %s", output)
	}
	if !strings.Contains(output, "exec-123") {
		t.Errorf("expected execution ID in output, got: %s", output)
	}
}

func TestRunCommand_MissingToken(t *testing.T) {
	resetViper()

	// No token set - set URL to avoid default issues
	viper.Set("url", "http://localhost:6161")
	viper.Set("token", "")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "API token not found") {
		t.Errorf("expected token error message, got: %s", output)
	}
}

func TestRunCommand_ServerError(t *testing.T) {
	resetViper()

	// Mock server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error (500)") {
		t.Errorf("expected error status in output, got: %s", output)
	}
}

func TestRunCommand_NotFoundError(t *testing.T) {
	resetViper()

	// Mock server returns 404 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Job not found"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "non-existent-job"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error (404)") {
		t.Errorf("expected 404 error in output, got: %s", output)
	}
}

func TestRunCommand_InvalidJSONResponse(t *testing.T) {
	resetViper()

	// Mock server returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-valid-json"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "failed to parse response") {
		t.Errorf("expected parse error message, got: %s", output)
	}
}

func TestRunCommand_Created201Response(t *testing.T) {
	resetViper()

	// Mock server returns 201 Created (also valid success status)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"execution_id": "exec-456",
		})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Execution started") {
		t.Errorf("expected success message for 201, got: %s", output)
	}
}

func TestRunCommand_RequiresJobIDArgument(t *testing.T) {
	resetViper()
	viper.Set("token", "test-token")

	var stderr bytes.Buffer
	rootCmd.SetOut(&stderr)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"run"}) // No job ID argument

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when no job ID provided")
	}
}

func TestRunCommand_UnauthorizedError(t *testing.T) {
	resetViper()

	// Mock server returns 401 Unauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid or expired token"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "invalid-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"run", "test-job-id"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error (401)") {
		t.Errorf("expected 401 error in output, got: %s", output)
	}
}
