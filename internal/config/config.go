// Package config handles environment variable loading for ports, database strings, etc.
package config

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
	// TODO: Load from environment variables
	return &Config{}, nil
}
