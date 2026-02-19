package main

import (
	"fmt"
	"os"

	"sophia/internal/cli"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	cli.SetBuildInfo(version, commit, buildDate)
	if err := cli.Execute(); err != nil {
		if !cli.IsHandledError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
