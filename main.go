package main

import (
	"errors"
	"os"

	"github.com/f24aalam/agentsync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		switch {
		case cmd.IsSilentError(err):
			os.Exit(1)
		case errors.Is(err, cmd.ErrUserAborted):
			os.Exit(130)
		default:
			cmd.PrintError(os.Stderr, err)
			os.Exit(1)
		}
	}
}
