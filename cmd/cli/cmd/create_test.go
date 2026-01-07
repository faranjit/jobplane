package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestCreateCommand_Success(t *testing.T) {
	resetViper()

	// Setup mock server that returns a successful job creation response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/jobs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got: %s", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if reqBody["name"] != "test-job" {
			t.Errorf("expected name=test-job, got %v", reqBody["name"])
		}
		if reqBody["image"] != "alpine:latest" {
			t.Errorf("expected image=alpine:latest, got %v", reqBody["image"])
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"job_id": "job-123",
		})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--image", "alpine:latest", "--command", "echo,hello"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Job created") {
		t.Errorf("expected success message, got: %s", output)
	}
	if !strings.Contains(output, "job-123") {
		t.Errorf("expected job ID in output, got: %s", output)
	}
}

func TestCreateCommand_WithTimeout(t *testing.T) {
	resetViper()

	var capturedTimeout float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		if timeout, ok := reqBody["default_timeout"]; ok {
			capturedTimeout = timeout.(float64)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"job_id": "job-456"})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "timeout-job", "--image", "alpine", "--command", "sleep,10", "--timeout", "300"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTimeout != 300 {
		t.Errorf("expected timeout=300, got %v", capturedTimeout)
	}
}

func TestCreateCommand_WithPriority(t *testing.T) {
	resetViper()

	var capturedPriority int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		if priority, ok := reqBody["priority"]; ok {
			fmt.Println(priority)
			capturedPriority = int(priority.(float64))
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"job_id": "job-456"})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "priority-job", "--image", "alpine", "--command", "sleep,10", "--priority", "61"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPriority != 61 {
		t.Errorf("expected priority=61, got %d", capturedPriority)
	}
}

func TestCreateCommand_MissingToken(t *testing.T) {
	resetViper()

	viper.Set("url", "http://localhost:6161")
	viper.Set("token", "")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "API token not found") {
		t.Errorf("expected token error message, got: %s", output)
	}
}

func TestCreateCommand_MissingName(t *testing.T) {
	resetViper()

	// Reset flags from previous tests
	createCmd.Flags().Set("name", "")
	createCmd.Flags().Set("image", "")
	createCmd.Flags().Set("command", "")

	// Use mock server that should NOT be called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when validation fails")
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "--name is required") {
		t.Errorf("expected name required error, got: %s", output)
	}
}

func TestCreateCommand_MissingImage(t *testing.T) {
	resetViper()

	// Reset flags from previous tests
	createCmd.Flags().Set("name", "")
	createCmd.Flags().Set("image", "")
	createCmd.Flags().Set("command", "")

	// Use mock server that should NOT be called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when validation fails")
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "--image is required") {
		t.Errorf("expected image required error, got: %s", output)
	}
}

func TestCreateCommand_ServerError(t *testing.T) {
	resetViper()

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
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error (500)") {
		t.Errorf("expected error status in output, got: %s", output)
	}
}

func TestCreateCommand_UnauthorizedError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Error (401)") {
		t.Errorf("expected 401 error in output, got: %s", output)
	}
}

func TestCreateCommand_Created201Response(t *testing.T) {
	resetViper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"job_id": "job-789",
		})
	}))
	defer server.Close()

	viper.Set("url", server.URL)
	viper.Set("token", "test-token")

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)
	rootCmd.SetArgs([]string{"create", "--name", "test-job", "--image", "alpine", "--command", "echo"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Job created") {
		t.Errorf("expected success message for 201, got: %s", output)
	}
}
