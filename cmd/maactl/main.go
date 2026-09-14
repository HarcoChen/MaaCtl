// Command maactl is the MaaFramework and ProjectInterface command-line client.
package main

import (
	"fmt"
	"os"

	"maactl/internal/cli"
)

// version is the local development version; release builds inject the tag
// version through -ldflags "-X main.version=<version>".
var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	root := cli.NewRootCommand(version)
	root.SetArgs(args)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.Execute()
}
