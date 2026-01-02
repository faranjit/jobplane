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

	// URL of the Control Plane (e.g., "http://localhost:8080")
	ControllerURL string
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

	return &Config{
		DatabaseURL:        dbUrl,
		HTTPPort:           port,
		WorkerConcurrency:  concurrency,
		WorkerPollInterval: pollInterval,
		ControllerURL:      controllerURL,
	}, nil
}
