package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	// Clear any existing env vars
	t.Setenv("DATABASE_URL", "")

	_, err := Load("")
	if err == nil {
		t.Error("expected error when DATABASE_URL is missing")
	}
	if err.Error() != "database_url is required (env: DATABASE_URL)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults
	if cfg.HTTPPort != 6161 {
		t.Errorf("expected HTTPPort 6161, got %d", cfg.HTTPPort)
	}
	if cfg.ControllerURL != "http://localhost:6161" {
		t.Errorf("expected ControllerURL http://localhost:6161, got %s", cfg.ControllerURL)
	}
	if cfg.HeartVisibilityExtension != 5*time.Minute {
		t.Errorf("expected HeartVisibilityExtension 5m, got %v", cfg.HeartVisibilityExtension)
	}
	if cfg.WorkerConcurrency != 1 {
		t.Errorf("expected WorkerConcurrency 1, got %d", cfg.WorkerConcurrency)
	}
	if cfg.WorkerPollInterval != 1*time.Second {
		t.Errorf("expected WorkerPollInterval 1s, got %v", cfg.WorkerPollInterval)
	}
	if cfg.WorkerMaxBackoff != 30*time.Second {
		t.Errorf("expected WorkerMaxBackoff 30s, got %v", cfg.WorkerMaxBackoff)
	}
	if cfg.WorkerHeartbeatInterval != 2*time.Minute {
		t.Errorf("expected WorkerHeartbeatInterval 2m, got %v", cfg.WorkerHeartbeatInterval)
	}
	if cfg.Runtime != "docker" {
		t.Errorf("expected Runtime docker, got %s", cfg.Runtime)
	}
	if cfg.OTELEndpoint != "localhost:4317" {
		t.Errorf("expected OTELEndpoint localhost:4317, got %s", cfg.OTELEndpoint)
	}
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://custom/db")
	t.Setenv("PORT", "9999")
	t.Setenv("WORKER_CONCURRENCY", "5")
	t.Setenv("WORKER_POLL_INTERVAL", "2s")
	t.Setenv("CONTROLLER_URL", "http://custom:8080")
	t.Setenv("RUNTIME", "exec")
	t.Setenv("RUNTIME_WORKDIR", "/tmp/jobs")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://custom/db" {
		t.Errorf("expected DatabaseURL from env, got %s", cfg.DatabaseURL)
	}
	if cfg.HTTPPort != 9999 {
		t.Errorf("expected HTTPPort 9999, got %d", cfg.HTTPPort)
	}
	if cfg.WorkerConcurrency != 5 {
		t.Errorf("expected WorkerConcurrency 5, got %d", cfg.WorkerConcurrency)
	}
	if cfg.WorkerPollInterval != 2*time.Second {
		t.Errorf("expected WorkerPollInterval 2s, got %v", cfg.WorkerPollInterval)
	}
	if cfg.ControllerURL != "http://custom:8080" {
		t.Errorf("expected ControllerURL http://custom:8080, got %s", cfg.ControllerURL)
	}
	if cfg.Runtime != "exec" {
		t.Errorf("expected Runtime exec, got %s", cfg.Runtime)
	}
	if cfg.RuntimeWorkDir != "/tmp/jobs" {
		t.Errorf("expected RuntimeWorkDir /tmp/jobs, got %s", cfg.RuntimeWorkDir)
	}
	if cfg.OTELEndpoint != "otel-collector:4317" {
		t.Errorf("expected OTELEndpoint otel-collector:4317, got %s", cfg.OTELEndpoint)
	}
}

func TestLoad_InvalidRuntime(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("RUNTIME", "invalid")

	_, err := Load("")
	if err == nil {
		t.Error("expected error for invalid runtime")
	}
}

func TestLoad_ConfigFile(t *testing.T) {
	// Create temp config file
	tmpFile, err := os.CreateTemp("", "jobplane-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `
database_url: "postgres://config-file/db"
http_port: 7777
worker_concurrency: 10
runtime: exec
`
	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Clear env vars that would override
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "")
	t.Setenv("WORKER_CONCURRENCY", "")
	t.Setenv("RUNTIME", "")

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://config-file/db" {
		t.Errorf("expected DatabaseURL from config file, got %s", cfg.DatabaseURL)
	}
	if cfg.HTTPPort != 7777 {
		t.Errorf("expected HTTPPort 7777, got %d", cfg.HTTPPort)
	}
	if cfg.WorkerConcurrency != 10 {
		t.Errorf("expected WorkerConcurrency 10, got %d", cfg.WorkerConcurrency)
	}
	if cfg.Runtime != "exec" {
		t.Errorf("expected Runtime exec, got %s", cfg.Runtime)
	}
}

func TestLoad_EnvOverridesConfigFile(t *testing.T) {
	// Create temp config file
	tmpFile, err := os.CreateTemp("", "jobplane-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `
database_url: "postgres://from-file/db"
http_port: 7777
`
	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Set env var to override config file
	t.Setenv("DATABASE_URL", "postgres://from-env/db")
	t.Setenv("PORT", "8888")

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Env should override config file
	if cfg.DatabaseURL != "postgres://from-env/db" {
		t.Errorf("expected DatabaseURL from env, got %s", cfg.DatabaseURL)
	}
	if cfg.HTTPPort != 8888 {
		t.Errorf("expected HTTPPort 8888 from env, got %d", cfg.HTTPPort)
	}
}

func TestLoad_InvalidConfigFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load("/nonexistent/path/to/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent config file")
	}
}
