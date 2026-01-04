package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "jobctl",
	Short: "Jobctl is a command line tool for interacting with the jobplane platform",
	Long: `jobctl is the command-line interface for the JobPlane distributed job execution platform.

JobPlane provides a multi-tenant platform for defining, scheduling, and executing 
background jobs in isolated runtime environments. The architecture follows a clear 
control plane / data plane separation:

  - Control Plane: Stateless HTTP API for job definitions and execution lifecycle
  - Data Plane: Workers that pull jobs from the queue and execute them

Common workflows:
  
  Create a job definition:
    jobctl create --name "my-job" --image "python:3.11" --command "python,script.py"

  Run an existing job:
    jobctl run <job-id>

  Create and run a job in one step:
    jobctl submit --name "quick-job" --image "alpine" --command "echo,hello"

  Check execution status:
    jobctl status <execution-id>

  Stream logs:
    jobctl logs <execution-id> --follow

Configuration:
  Set the API endpoint and credentials via environment variables or a config file:
    JOBPLANE_API_URL    API endpoint (default: http://localhost:6161)
    JOBPLANE_TOKEN    	Tenant API Token for authentication

For more information, visit: https://github.com/faranjit/jobplane`,
}

func Execute() error {
	return rootCmd.Execute()
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".jobctl"
		viper.AddConfigPath(home)
		viper.SetConfigName(".jobctl")
		viper.SetConfigType("yaml")
	}

	// Read environment variables that match "JOBPLANE_VARNAME"
	viper.SetEnvPrefix("JOBPLANE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.jobctl.yaml)")

	rootCmd.PersistentFlags().String("url", "http://localhost:6161", "JobPlane Controller URL")
	viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))

	rootCmd.PersistentFlags().StringP("token", "t", "", "API Token for authentication")
	viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
}
