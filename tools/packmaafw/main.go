// Command packmaafw builds the MaaFramework payload that bundled maactl builds
// carry inside the executable.
//
// It packs the MaaFramework runtime that a release archive unpacks into
// maafw/bin and writes it to internal/maafw/bundled/payload, where the next
// `go build -tags bundled ./cmd/maactl` embeds it.
//
// The tool never downloads anything: unpack the MAA-<platform>-<version>.zip
// archive of the platform you are building for into maafw/ first (CI does this
// per platform). Packing fails when the directory holds no runtime, or when its
// libraries belong to another platform, so a mis-configured build cannot
// silently produce an executable that cannot load MaaFramework.
//
//	go run ./tools/packmaafw                      # maafw/bin, version pinned in maafw.version
//	go run ./tools/packmaafw -dir /tmp/maa/bin    # runtime unpacked elsewhere
//	go run ./tools/packmaafw -platform win-x86_64 -version v5.13.1   # assert both (CI does this)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"maactl/internal/maafw/pack"
	"maactl/internal/platform"
)

const (
	defaultOut    = "internal/maafw/bundled/payload"
	versionFile   = "maafw.version"
	containerName = "bin.zip"
	versionName   = "version.txt"
	platformName  = "platform.txt"
)

type options struct {
	dir      string
	out      string
	platform string
	version  string
}

func main() {
	opts := parseFlags()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	target, err := expectedTarget(opts.platform)
	if err != nil {
		return err
	}
	if err := checkRuntime(opts.dir, target); err != nil {
		return err
	}
	version, err := resolveVersion(opts.version)
	if err != nil {
		return err
	}
	fmt.Printf("packing MaaFramework %s for %s from %s\n", version, target.ID(), opts.dir)

	container, files, err := pack.Container(opts.dir, target)
	if err != nil {
		return err
	}
	if err := writePayload(opts.out, container, version, target); err != nil {
		return err
	}
	fmt.Printf("  payload: %s (%d files, %s)\n", filepath.Join(opts.out, containerName), files, humanBytes(int64(len(container))))
	fmt.Printf("  version: %s\n", filepath.Join(opts.out, versionName))
	fmt.Printf("  platform: %s\n", filepath.Join(opts.out, platformName))
	fmt.Printf("build a self-contained executable with: go build -tags bundled -o %s ./cmd/maactl\n", target.Executable("maactl"))
	return nil
}

// expectedTarget resolves the platform being built. The payload is embedded in a
// native build of maactl, so it can only ever be the platform this tool runs on;
// -platform therefore asserts that the runner really is the one the build
// matrix expected.
func expectedTarget(value string) (platform.Target, error) {
	host := platform.Host()
	if !host.Known() {
		return platform.Target{}, fmt.Errorf("maactl is not built for %s/%s; supported platforms: %s", runtime.GOOS, runtime.GOARCH, platform.IDs())
	}
	if strings.TrimSpace(value) == "" {
		return host, nil
	}
	requested, err := platform.Parse(value)
	if err != nil {
		return platform.Target{}, err
	}
	if requested != host {
		return platform.Target{}, fmt.Errorf("-platform %s does not match this tool's platform %s; pack and build for %s on a %s machine", requested.ID(), host.ID(), requested.ID(), requested.Title())
	}
	return requested, nil
}

// checkRuntime verifies that dir holds the runtime of target, so the payload
// never mixes a platform's libraries with another platform's build.
func checkRuntime(dir string, target platform.Target) error {
	detected, err := pack.Inspect(dir)
	if err != nil {
		return fmt.Errorf("%v; for %s, %s must hold %s, so unpack the MAA-%s release archive there",
			err, target.ID(), dir, strings.Join(target.Libraries(), " and "), target.ID())
	}
	if detected.OS != target.OS || (detected.Arch != "" && detected.Arch != target.Arch) {
		return fmt.Errorf("%s holds %s libraries but %s was requested; unpack the MAA-%s release archive there instead",
			dir, detected.ID(), target.ID(), target.ID())
	}
	if detected.Arch == "" {
		fmt.Fprintf(os.Stderr, "warning: cannot read the architecture of %s; assuming %s\n", filepath.Join(dir, target.FrameworkLibrary()), target.Arch)
	}
	return nil
}

// resolveVersion reads the MaaFramework version from -version or from the
// pinned maafw.version file.
func resolveVersion(value string) (string, error) {
	version := strings.TrimSpace(value)
	if version == "" {
		pinned, err := os.ReadFile(versionFile)
		if err != nil {
			return "", fmt.Errorf("no -version and cannot read %s: %w", versionFile, err)
		}
		version = strings.TrimSpace(string(pinned))
	}
	if version == "" {
		return "", fmt.Errorf("%s is empty; write the MaaFramework release tag into it, e.g. v5.13.1", versionFile)
	}
	// Accept both "5.13.1" and the tagged form "v5.13.1".
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version, nil
}

// writePayload replaces the generated payload files, keeping the hand-written
// README that makes the directory embeddable for unbundled builds.
func writePayload(dir string, container []byte, version string, target platform.Target) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	files := []struct {
		name    string
		content []byte
	}{
		{containerName, container},
		{versionName, []byte(version + "\n")},
		{platformName, []byte(target.ID() + "\n")},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file.name), file.content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func parseFlags() options {
	var opts options
	flags := flag.NewFlagSet("packmaafw", flag.ExitOnError)
	flags.StringVar(&opts.dir, "dir", pack.Dir, "MaaFramework runtime directory, as unpacked from a release archive")
	flags.StringVar(&opts.out, "out", defaultOut, "directory receiving "+containerName+", "+versionName+", and "+platformName)
	flags.StringVar(&opts.platform, "platform", "", "assert the platform being packed, e.g. win-x86_64 (default: the platform this tool runs on)")
	flags.StringVar(&opts.version, "version", "", "MaaFramework release tag, e.g. v5.13.1 (default: read from "+versionFile+")")
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
