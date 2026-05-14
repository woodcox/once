package main

import (
	"os"

	"github.com/woodcox/once/internal/command"
	"github.com/woodcox/once/internal/logging"
)

func main() {
	logging.SetupStderr()

	if err := command.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
