// Command maactl is the MaaFramework and ProjectInterface command-line client.
package main

import (
	"errors"
	"fmt"
	"os"

	"maactl/internal/cli"
)

// version is the local development version; release builds inject the tag
// version through -ldflags "-X main.version=<version>".
var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes the command tree and maps failures onto the documented exit
// codes: a cli.ExitError carries its own code, anything else is an internal
// error.
func run(args []string) int {
	root := cli.NewRootCommand(version)
	root.SetArgs(cli.NormalizeArgs(args))
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	err := root.Execute()
	if err == nil {
		return cli.ExitOK
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	var exit *cli.ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return cli.ExitInternal
}
