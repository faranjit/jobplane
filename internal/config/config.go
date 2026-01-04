// Package config handles environment variable loading for ports, database strings, etc.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values for the application.
type Config struct {
	// Database connection string
	DatabaseURL string

	// HTTP server port for the controller
	HTTPPort int

	// Worker-specific configuration
	WorkerConcurrency int

	// Worker Poll Interval
	WorkerPollInterval time.Duration

	// Maximum backoff when queue is empty
	WorkerMaxBackoff time.Duration

	// Heartbeat interval during job execution
	WorkerHeartbeatInterval time.Duration

	// How long to extend visibility timeout on each heartbeat
	WorkerVisibilityExtension time.Duration

	// URL of the Control Plane (e.g., "http://localhost:8080")
	ControllerURL string

	// Runtime type: "docker" (default) or "exec"
	Runtime string

	// WorkDir for exec runtime (optional, defaults to system temp)
	RuntimeWorkDir string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	portStr := os.Getenv("PORT")
	port := 6161 // Default
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		port = p
	}

	// Worker Concurrency
	concurrencyStr := os.Getenv("WORKER_CONCURRENCY")
	concurrency := 1 // Default
	if concurrencyStr != "" {
		c, err := strconv.Atoi(concurrencyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_CONCURRENCY: %w", err)
		}
		concurrency = c
	}

	// Worker Poll Interval
	pollIntervalStr := os.Getenv("WORKER_POLL_INTERVAL")
	pollInterval := 1 * time.Second // Default
	if pollIntervalStr != "" {
		pi, err := time.ParseDuration(pollIntervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_POLL_INTERVAL: %w", err)
		}
		pollInterval = pi
	}

	controllerURL := os.Getenv("CONTROLLER_URL")
	if controllerURL == "" {
		controllerURL = "http://localhost:6161"
	}

	// Worker Max Backoff
	maxBackoffStr := os.Getenv("WORKER_MAX_BACKOFF")
	maxBackoff := 30 * time.Second // Default
	if maxBackoffStr != "" {
		mb, err := time.ParseDuration(maxBackoffStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MAX_BACKOFF: %w", err)
		}
		maxBackoff = mb
	}

	// Worker Heartbeat Interval
	heartbeatIntervalStr := os.Getenv("WORKER_HEARTBEAT_INTERVAL")
	heartbeatInterval := 2 * time.Minute // Default
	if heartbeatIntervalStr != "" {
		hi, err := time.ParseDuration(heartbeatIntervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_HEARTBEAT_INTERVAL: %w", err)
		}
		heartbeatInterval = hi
	}

	// Worker Visibility Extension
	visibilityExtensionStr := os.Getenv("WORKER_VISIBILITY_EXTENSION")
	visibilityExtension := 5 * time.Minute // Default
	if visibilityExtensionStr != "" {
		ve, err := time.ParseDuration(visibilityExtensionStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_VISIBILITY_EXTENSION: %w", err)
		}
		visibilityExtension = ve
	}

	// Runtime selection
	runtimeType := os.Getenv("RUNTIME")
	if runtimeType == "" {
		runtimeType = "docker" // Default
	}
	if runtimeType != "docker" && runtimeType != "exec" {
		return nil, fmt.Errorf("invalid RUNTIME: must be 'docker' or 'exec', got '%s'", runtimeType)
	}

	runtimeWorkDir := os.Getenv("RUNTIME_WORKDIR") // Optional, exec runtime only

	return &Config{
		DatabaseURL:               dbUrl,
		HTTPPort:                  port,
		WorkerConcurrency:         concurrency,
		WorkerPollInterval:        pollInterval,
		WorkerMaxBackoff:          maxBackoff,
		WorkerHeartbeatInterval:   heartbeatInterval,
		WorkerVisibilityExtension: visibilityExtension,
		ControllerURL:             controllerURL,
		Runtime:                   runtimeType,
		RuntimeWorkDir:            runtimeWorkDir,
	}, nil
}
