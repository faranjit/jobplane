// Package config handles configuration loading from files and environment variables.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration values for the application.
type Config struct {
	// System Secret
	SystemSecret string `mapstructure:"system_secret"`

	// Database connection string
	DatabaseURL string `mapstructure:"database_url"`

	// HTTP server port for the controller
	HTTPPort int `mapstructure:"http_port"`

	// URL of the Control Plane (e.g., "http://localhost:6161")
	ControllerURL string `mapstructure:"controller_url"`

	// How long to extend visibility timeout on each heartbeat
	HeartVisibilityExtension time.Duration `mapstructure:"heartbeat_visibility_extension"`

	// Worker-specific configuration
	WorkerConcurrency int `mapstructure:"worker_concurrency"`

	// Worker Poll Interval
	WorkerPollInterval time.Duration `mapstructure:"worker_poll_interval"`

	// Maximum backoff when queue is empty
	WorkerMaxBackoff time.Duration `mapstructure:"worker_max_backoff"`

	// Heartbeat interval during job execution
	WorkerHeartbeatInterval time.Duration `mapstructure:"worker_heartbeat_interval"`

	// Runtime type: "docker" (default), "exec", or "kubernetes"
	Runtime string `mapstructure:"runtime"`

	// WorkDir for exec runtime (optional, defaults to system temp)
	RuntimeWorkDir string `mapstructure:"runtime_workdir"`

	// Kubernetes runtime configuration
	KubernetesNamespace      string `mapstructure:"kubernetes_namespace"`
	KubernetesServiceAccount string `mapstructure:"kubernetes_service_account"`
	KubernetesCPULimit       string `mapstructure:"kubernetes_cpu_limit"`
	KubernetesMemoryLimit    string `mapstructure:"kubernetes_memory_limit"`

	// OpenTelemetry collector endpoint
	OTELEndpoint string `mapstructure:"otel_endpoint"`
}

// Load reads configuration from file (if provided) and environment variables.
// Environment variables take precedence over config file values.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("http_port", 6161)
	v.SetDefault("controller_url", "http://localhost:6161")
	v.SetDefault("heartbeat_visibility_extension", "5m")
	v.SetDefault("worker_concurrency", 1)
	v.SetDefault("worker_poll_interval", "1s")
	v.SetDefault("worker_max_backoff", "30s")
	v.SetDefault("worker_heartbeat_interval", "2m")
	v.SetDefault("runtime", "docker")
	v.SetDefault("otel_endpoint", "localhost:4317")

	// Read config file if specified
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		// Look for jobplane.yaml in current directory
		v.SetConfigName("jobplane")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		// Ignore error if file doesn't exist
		_ = v.ReadInConfig()
	}

	// Bind environment variables
	// Map env var names to config keys
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Manual bindings for backward compatibility
	_ = v.BindEnv("system_secret", "SYSTEM_SECRET")
	_ = v.BindEnv("database_url", "DATABASE_URL")
	_ = v.BindEnv("http_port", "PORT")
	_ = v.BindEnv("controller_url", "CONTROLLER_URL")
	_ = v.BindEnv("heartbeat_visibility_extension", "HEARTBEAT_VISIBILITY_EXTENSION")
	_ = v.BindEnv("worker_concurrency", "WORKER_CONCURRENCY")
	_ = v.BindEnv("worker_poll_interval", "WORKER_POLL_INTERVAL")
	_ = v.BindEnv("worker_max_backoff", "WORKER_MAX_BACKOFF")
	_ = v.BindEnv("worker_heartbeat_interval", "WORKER_HEARTBEAT_INTERVAL")
	_ = v.BindEnv("runtime", "RUNTIME")
	_ = v.BindEnv("runtime_workdir", "RUNTIME_WORKDIR")
	_ = v.BindEnv("kubernetes_namespace", "KUBERNETES_NAMESPACE")
	_ = v.BindEnv("kubernetes_service_account", "KUBERNETES_SERVICE_ACCOUNT")
	_ = v.BindEnv("kubernetes_cpu_limit", "KUBERNETES_CPU_LIMIT")
	_ = v.BindEnv("kubernetes_memory_limit", "KUBERNETES_MEMORY_LIMIT")
	_ = v.BindEnv("otel_endpoint", "OTEL_EXPORTER_OTLP_ENDPOINT")

	// Validate required fields
	if v.GetString("database_url") == "" {
		return nil, fmt.Errorf("database_url is required (env: DATABASE_URL)")
	}
	if v.GetString("system_secret") == "" {
		return nil, fmt.Errorf("system_secret is required (env: SYSTEM_SECRET)")
	}

	// Parse config
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate runtime
	if cfg.Runtime != "docker" && cfg.Runtime != "exec" && cfg.Runtime != "kubernetes" {
		return nil, fmt.Errorf("invalid runtime: must be 'docker', 'exec', or 'kubernetes', got '%s'", cfg.Runtime)
	}

	if cfg.HeartVisibilityExtension <= 0 {
		cfg.HeartVisibilityExtension = 5 * time.Minute
	}

	return cfg, nil
}
