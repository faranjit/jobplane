// Package main is the entry point for the jobplane CLI.
// The CLI is the developer terminal tool for interacting with the jobplane API.
package main

import (
	"jobplane/cmd/cli/cmd"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
