package main

import (
	"fmt"
	"os"

	"sophia/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if !cli.IsHandledError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
