// Command packmaafw builds the MaaFramework payload that bundled maactl builds
// carry inside the executable.
//
// It packs the runtime that a MaaFramework release archive unpacks into
// maafw/bin and writes it to internal/maafw/bundled/payload, where the next
// `go build -tags bundled ./cmd/maactl` embeds it.
//
// The tool is deliberately unaware of MaaFramework versions and platforms: it
// downloads nothing, asserts nothing, and packs whatever files it finds in the
// directory. Filling that directory with the runtime of the platform being
// built for is the job of CI (.github/scripts/fetch_maafw.py verifies the
// archive it unpacks); maactl itself reads the version from the loaded runtime
// at run time, so bumping MaaFramework never touches this code.
//
//	go run ./tools/packmaafw                      # packs maafw/bin
//	go run ./tools/packmaafw -dir /tmp/maa/bin    # packs a runtime kept elsewhere
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"maactl/internal/maafw/pack"
)

const (
	defaultOut    = "internal/maafw/bundled/payload"
	containerName = "bin.zip"
	// keepName is the hand-written README that keeps the directory embeddable
	// for unbundled builds; every other file there is generated.
	keepName = "README.md"
)

type options struct {
	dir string
	out string
}

func main() {
	opts := parseFlags()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	container, files, err := pack.Container(opts.dir)
	if err != nil {
		return err
	}
	if err := writePayload(opts.out, container); err != nil {
		return err
	}
	payload := filepath.Join(opts.out, containerName)
	fmt.Printf("packed %d files from %s\n", files, opts.dir)
	fmt.Printf("  payload: %s (%s)\n", payload, humanBytes(int64(len(container))))
	fmt.Printf("  MaaFramework version: read from the runtime at run time\n")
	fmt.Printf("build a self-contained executable with: go build -tags bundled -o %s ./cmd/maactl\n",
		executable("maactl"))
	return nil
}

// writePayload replaces the generated payload with container, keeping the
// hand-written README that makes the directory embeddable for unbundled builds.
// Generated files removed in favour of the container include the version.txt and
// platform.txt older packers wrote: the payload is a plain archive of the
// runtime, and nothing else.
func writePayload(dir string, container []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == keepName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, containerName), container, 0o644)
}

// executable appends the host platform's executable suffix.
func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func parseFlags() options {
	var opts options
	flags := flag.NewFlagSet("packmaafw", flag.ExitOnError)
	flags.StringVar(&opts.dir, "dir", pack.Dir, "MaaFramework runtime directory, as unpacked from a release archive")
	flags.StringVar(&opts.out, "out", defaultOut, "directory receiving "+containerName)
	flags.Parse(os.Args[1:])
	return opts
}

// humanBytes formats a byte count with a binary unit suffix.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < len("KMGT")-1 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
