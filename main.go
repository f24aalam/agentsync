package main

import (
	"os"

	"github.com/f24aalam/agentsync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
