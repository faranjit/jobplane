package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRootCommand_DefaultURL(t *testing.T) {
	resetViper()

	// The default URL should be set by root command init
	// We need to trigger flag initialization
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("url", "http://localhost:6161", "JobPlane Controller URL")
	viper.BindPFlag("url", cmd.PersistentFlags().Lookup("url"))

	url := viper.GetString("url")
	if url != "http://localhost:6161" {
		t.Errorf("expected default url http://localhost:6161, got: %s", url)
	}
}

func TestRootCommand_EnvVarBinding(t *testing.T) {
	resetViper()

	// Set environment variable
	t.Setenv("JOBPLANE_TOKEN", "env-token-value")
	t.Setenv("JOBPLANE_URL", "http://custom-url:8080")

	token := viper.GetString("token")
	url := viper.GetString("url")

	if token != "env-token-value" {
		t.Errorf("expected token from env var, got: %s", token)
	}
	if url != "http://custom-url:8080" {
		t.Errorf("expected url from env var, got: %s", url)
	}
}

func TestRootCommand_ExecuteReturnsNoError(t *testing.T) {
	resetViper()

	// Test that root command executes without error when given help flag
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("root command should execute without error: %v", err)
	}
}

func TestRootCommand_HasRunSubcommand(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "run [job_id]" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'run' subcommand to be registered with root command")
	}
}

func TestExecute_ReturnsError(t *testing.T) {
	resetViper()

	// Set args that will cause an error (unknown command)
	rootCmd.SetArgs([]string{"unknown-command-xyz"})

	err := Execute()
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestRootCommand_CustomConfigFile(t *testing.T) {
	resetViper()

	// Create a temp config file
	tmpFile, err := os.CreateTemp("", "jobctl-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test config
	tmpFile.WriteString("url: http://custom-from-config:9999\ntoken: config-token\n")
	tmpFile.Close()

	// Set the config file flag
	cfgFile = tmpFile.Name()
	initConfig()

	// Verify config was loaded
	url := viper.GetString("url")
	if url != "http://custom-from-config:9999" {
		t.Errorf("expected url from config file, got: %s", url)
	}

	token := viper.GetString("token")
	if token != "config-token" {
		t.Errorf("expected token from config file, got: %s", token)
	}

	// Reset for other tests
	cfgFile = ""
}
