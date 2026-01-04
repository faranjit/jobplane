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

func TestSubmitCommand_Success(t *testing.T) {
	resetViper()

	createCalled := false
	runCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs" && r.Method == http.MethodPost {
			createCalled = true
			// Verify request body
			var reqBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&reqBody)
			if reqBody["name"] != "test-job" {
				t.Errorf("expected name=test-job, got %v", reqBody["name"])
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"job_id": "job-123"})
			return
		}

		if strings.HasSuffix(r.URL.Path, "/run") && r.Method == http.MethodPost {
			runCalled = true
			if !strings.Contains(r.URL.Path, "job-123") {
				t.Errorf("expected job-123 in run path, got %s", r.URL.Path)
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"execution_id": "exec-456"})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "test-job", "--image", "alpine", "--command", "echo,hello"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !createCalled {
		t.Error("expected create endpoint to be called")
	}
	if !runCalled {
		t.Error("expected run endpoint to be called")
	}

	output := stdout.String()
	if !strings.Contains(output, "Job submitted") {
		t.Errorf("expected success message, got: %s", output)
	}
	if !strings.Contains(output, "job-123") {
		t.Errorf("expected job ID in output, got: %s", output)
	}
	if !strings.Contains(output, "exec-456") {
		t.Errorf("expected execution ID in output, got: %s", output)
	}
}

func TestSubmitCommand_MissingToken(t *testing.T) {
	resetViper()

	viper.Set("url", "http://localhost:6161")
	viper.Set("token", "")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "API token not found") {
		t.Errorf("expected token error message, got: %s", output)
	}
}

func TestSubmitCommand_MissingName(t *testing.T) {
	resetViper()

	// Reset flags from previous tests
	submitCmd.Flags().Set("name", "")
	submitCmd.Flags().Set("image", "")
	submitCmd.Flags().Set("command", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when validation fails")
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "--name is required") {
		t.Errorf("expected name required error, got: %s", output)
	}
}

func TestSubmitCommand_CreateFails(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Create failed") {
		t.Errorf("expected create failed message, got: %s", output)
	}
}

func TestSubmitCommand_RunFails(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"job_id": "job-789"})
			return
		}

		// Run fails
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Run failed"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "job-789") {
		t.Errorf("expected job ID in error output, got: %s", output)
	}
	if !strings.Contains(output, "run failed") {
		t.Errorf("expected run failed message, got: %s", output)
	}
}

func TestSubmitCommand_WithTimeout(t *testing.T) {
	resetViper()

	var capturedTimeout float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs" {
			var reqBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&reqBody)
			if timeout, ok := reqBody["default_timeout"]; ok {
				capturedTimeout = timeout.(float64)
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"job_id": "job-timeout"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"execution_id": "exec-timeout"})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "timeout-job", "--image", "alpine", "--command", "sleep,10", "--timeout", "600"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTimeout != 600 {
		t.Errorf("expected timeout=600, got %v", capturedTimeout)
	}
}

func TestSubmitCommand_UnauthorizedError(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid token"))
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "invalid-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"submit", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Create failed (401)") {
		t.Errorf("expected 401 error in output, got: %s", output)
	}
}
