// Command fallbakit is the developer CLI for the Fallbakit platform.
package main

import (
	"os"

	"github.com/fallbakit/cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
