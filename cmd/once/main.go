package main

import (
	"os"

	"github.com/woodcox/once/internal/command"
	"github.com/woodcox/once/internal/logging"
)

// main is the program entry point. It configures logging to stderr, creates and executes the CLI root command, and exits with code 1 if command execution fails.
func main() {
	logging.SetupStderr()

	if err := command.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
