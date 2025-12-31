// Package config handles environment variable loading for ports, database strings, etc.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration values for the application.
type Config struct {
	// Database connection string
	DatabaseURL string

	// HTTP server port for the controller
	HTTPPort int

	// Worker-specific configuration
	WorkerConcurrency int
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

	concurrencyStr := os.Getenv("WORKER_CONCURRENCY")
	concurrency := 5 // Default
	if concurrencyStr != "" {
		c, err := strconv.Atoi(concurrencyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_CONCURRENCY: %w", err)
		}
		concurrency = c
	}

	return &Config{
		DatabaseURL:       dbUrl,
		HTTPPort:          port,
		WorkerConcurrency: concurrency,
	}, nil
}
