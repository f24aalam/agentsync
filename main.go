package main

import (
	"fmt"
	"os"

	"github.com/f24aalam/agentsync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		if !cmd.IsSilentError(err) {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
