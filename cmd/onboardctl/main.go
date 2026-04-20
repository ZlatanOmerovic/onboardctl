// Command onboardctl is the entrypoint for the onboardctl CLI.
//
// See internal/cli for the cobra command tree.
package main

import (
	"os"

	"github.com/ZlatanOmerovic/onboardctl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// cobra already prints the error; we only propagate the exit code.
		os.Exit(1)
	}
}
