// Command packmaafw builds the MaaFramework payload that bundled maactl builds
// carry inside the executable.
//
// It takes an official MaaFramework release archive and keeps only the files
// below the archive's bin/ directory, which is all maactl needs at run time. The
// result is written to internal/maafw/bundled/payload and embedded by the next
// `go build -tags bundled ./cmd/maactl`.
//
// An archive that is already on disk always wins over downloading it, so a local
// build works offline:
//
//	go run ./tools/packmaafw                       # uses assets/<archive>, version pinned in maafw.version
//	go run ./tools/packmaafw -archive assets/MAA-win-x86_64-v5.13.0.zip
//	go run ./tools/packmaafw -version v5.13.0      # downloads and caches in assets/
//
// Only when no archive is present does the tool download the release named by
// maafw.version (or -version) and cache it in assets/ for later runs.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maactl/internal/maafw/pack"
)

const (
	defaultRepo     = "MaaXYZ/MaaFramework"
	defaultPlatform = "win-x86_64"
	defaultOut      = "internal/maafw/bundled/payload"
	assetsDir       = "assets"
	versionFile     = "maafw.version"
	containerName   = "bin.zip"
	versionName     = "version.txt"

	// downloadAttempts bounds retries of a single download.
	downloadAttempts = 3

	// downloadStallTimeout aborts a download that stops making progress, so a
	// dead connection fails fast and is retried instead of hanging forever.
	downloadStallTimeout = 60 * time.Second

	// acceptJSON selects the GitHub API representation, acceptBinary the raw
	// asset bytes, and acceptAny leaves the choice to the server.
	acceptJSON   = "application/vnd.github+json"
	acceptBinary = "application/octet-stream"
	acceptAny    = ""
)

// httpClient bounds the download time instead of relying on a per-read timeout.
var httpClient = &http.Client{Timeout: 30 * time.Minute}

type options struct {
	archive  string
	version  string
	platform string
	repo     string
	assets   string
	out      string
}

func main() {
	opts := parseFlags()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	archive, version, source, err := loadArchive(opts)
	if err != nil {
		return err
	}
	fmt.Printf("packing MaaFramework %s from %s\n", version, source)
	container, files, err := pack.Container(archive)
	if err != nil {
		return err
	}
	if err := writePayload(opts.out, container, version); err != nil {
		return err
	}
	fmt.Printf("  payload: %s (%d files, %s)\n", filepath.Join(opts.out, containerName), files, humanBytes(int64(len(container))))
	fmt.Printf("  version: %s\n", filepath.Join(opts.out, versionName))
	fmt.Println("build a self-contained executable with: go build -tags bundled -o maactl.exe ./cmd/maactl")
	return nil
}

// loadArchive returns the bytes of a MaaFramework release archive, the version
// it belongs to, and a description of where it came from.
//
// An archive that is already on disk always wins, so a local build never hits
// the network when the release archive is there. Only when none is available is
// the release downloaded (and then cached for later runs).
func loadArchive(opts options) ([]byte, string, string, error) {
	// 1. An explicitly named archive, by path or URL.
	if opts.archive != "" && !isURL(opts.archive) {
		data, err := os.ReadFile(opts.archive)
		if err != nil {
			return nil, "", "", err
		}
		version := normalizeVersion(firstNonEmpty(opts.version, pack.VersionFromName(opts.archive)))
		if version == "" {
			return nil, "", "", fmt.Errorf("cannot tell the MaaFramework version of %q; pass -version", opts.archive)
		}
		return data, version, opts.archive, nil
	}

	// 2. An archive downloaded earlier into the assets directory.
	version := normalizeVersion(firstNonEmpty(opts.version, pinnedVersion()))
	if local := findArchive(opts, version); local != "" {
		data, err := os.ReadFile(local)
		if err != nil {
			return nil, "", "", err
		}
		if version = normalizeVersion(firstNonEmpty(version, pack.VersionFromName(local))); version == "" {
			return nil, "", "", fmt.Errorf("cannot tell the MaaFramework version of %q; pass -version", local)
		}
		return data, version, local, nil
	}

	// 3. An explicitly named URL, or the official release.
	if version == "" {
		return nil, "", "", fmt.Errorf("no MaaFramework archive in %s and no version to download; pass -archive <path> or pin %s", opts.assets, versionFile)
	}
	url := opts.archive
	if url == "" {
		url = releaseURL(opts.repo, opts.platform, version)
	}
	data, err := downloadReleaseURL(opts, url, version)
	if err != nil {
		return nil, "", "", err
	}
	// Cache the archive so later runs and offline builds reuse it.
	cached := filepath.Join(opts.assets, archiveName(opts.platform, version))
	if err := os.MkdirAll(opts.assets, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot create %s: %v\n", opts.assets, err)
	} else if err := os.WriteFile(cached, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot cache %s: %v\n", cached, err)
	} else {
		fmt.Printf("  cached: %s (%s)\n", cached, humanBytes(int64(len(data))))
	}
	return data, version, url, nil
}

// downloadReleaseURL downloads an explicit URL or the official release asset.
func downloadReleaseURL(opts options, url, version string) ([]byte, error) {
	if opts.archive != "" {
		return download(url, acceptAny)
	}
	return downloadRelease(opts.repo, opts.platform, version)
}

// findArchive returns a release archive already present in the assets
// directory, preferring the canonical file name and then the newest file.
func findArchive(opts options, version string) string {
	entries, err := os.ReadDir(opts.assets)
	if err != nil {
		return ""
	}
	canonical := filepath.Join(opts.assets, archiveName(opts.platform, version))
	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
			continue
		}
		name := entry.Name()
		if !strings.Contains(name, opts.platform) {
			continue
		}
		if version != "" && !strings.Contains(name, version) {
			continue
		}
		candidates = append(candidates, filepath.Join(opts.assets, name))
	}
	// A single archive is unambiguous even when it was renamed.
	if len(candidates) == 0 && version == "" && len(entries) == 1 {
		name := filepath.Join(opts.assets, entries[0].Name())
		if !entries[0].IsDir() && strings.HasSuffix(strings.ToLower(entries[0].Name()), ".zip") {
			candidates = []string{name}
		}
	}
	best := ""
	for _, candidate := range candidates {
		if candidate == canonical {
			return candidate
		}
		if best == "" || newer(candidate, best) {
			best = candidate
		}
	}
	return best
}

// newer reports whether a is more recently modified than b.
func newer(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil {
		return false
	}
	if errB != nil {
		return true
	}
	return infoA.ModTime().After(infoB.ModTime())
}

// pinnedVersion reads the MaaFramework version pinned in maafw.version and
// returns "" when the file is missing or empty.
func pinnedVersion() string {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return normalizeVersion(strings.TrimSpace(string(data)))
}

// writePayload replaces the generated payload files, keeping the hand-written
// README that makes the directory embeddable for unbundled builds.
func writePayload(dir string, container []byte, version string) error {
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
	if err := os.WriteFile(filepath.Join(dir, containerName), container, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, versionName), []byte(version+"\n"), 0o644)
}

// downloadRelease fetches a MaaFramework release asset.
//
// The asset is resolved through the GitHub API first: that way the exact asset
// name is used, and the download itself goes through api.github.com, which
// stays reachable in networks that block github.com. The conventional release
// download URL is the fallback.
func downloadRelease(repo, platform, version string) ([]byte, error) {
	var failures []error
	if asset, err := lookupAsset(repo, platform, version); err != nil {
		failures = append(failures, err)
	} else if data, err := download(assetEndpoint(repo, asset.ID), acceptBinary); err != nil {
		failures = append(failures, err)
	} else {
		return data, nil
	}
	if data, err := download(releaseURL(repo, platform, version), acceptAny); err != nil {
		failures = append(failures, err)
	} else {
		return data, nil
	}
	return nil, fmt.Errorf("download MaaFramework %s: %w", version, errors.Join(failures...))
}

// releaseAsset is the part of a GitHub release asset maactl needs.
type releaseAsset struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// lookupAsset asks the GitHub API which archive belongs to a release.
func lookupAsset(repo, platform, version string) (releaseAsset, error) {
	data, err := download(fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, version), acceptJSON)
	if err != nil {
		return releaseAsset{}, err
	}
	var rel release
	if err := json.Unmarshal(data, &rel); err != nil {
		return releaseAsset{}, fmt.Errorf("parse release %s: %w", version, err)
	}
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, platform) && strings.HasSuffix(asset.Name, ".zip") {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release %s has no %s archive", version, platform)
}

// assetEndpoint is the API download endpoint of an asset. It serves the bytes
// itself instead of redirecting to github.com.
func assetEndpoint(repo string, id int) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, id)
}

// download fetches url into memory, reporting progress on stderr. Requests are
// retried because release downloads occasionally fail with transient errors.
func download(url, accept string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		data, err := fetch(url, accept)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if attempt == downloadAttempts || !retryable(err) {
			break
		}
		fmt.Fprintf(os.Stderr, "  %v; retrying (%d/%d)\n", err, attempt+1, downloadAttempts)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return nil, lastErr
}

// statusError reports a response that was not a success.
type statusError struct {
	url    string
	status string
	code   int
}

func (e *statusError) Error() string { return fmt.Sprintf("GET %s: %s", e.url, e.status) }

// retryable reports whether a failed request is worth repeating. Network errors
// are usually transient, while most client errors are not.
func retryable(err error) bool {
	var status *statusError
	if errors.As(err, &status) {
		return status.code >= 500 || status.code == http.StatusTooManyRequests
	}
	return true
}

// fetch performs one download attempt and reports progress on stderr. A GitHub
// token from GH_TOKEN or GITHUB_TOKEN lifts the API rate limit.
func fetch(url, accept string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token := githubToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &statusError{url: url, status: response.Status, code: response.StatusCode}
	}
	body := &progressReader{reader: response.Body, total: response.ContentLength}
	defer body.done()
	// Watchdog: closing the body turns a stalled read into an error instead of
	// blocking forever, so the request can be retried.
	stalled := time.AfterFunc(downloadStallTimeout, func() { response.Body.Close() })
	defer stalled.Stop()
	body.onRead = func() { stalled.Reset(downloadStallTimeout) }
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return data, nil
}

// progressReader reports download progress on stderr in 5% steps and notifies a
// watchdog on every read.
type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	step   int
	onRead func()
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.reader.Read(b)
	p.read += int64(n)
	if n > 0 && p.onRead != nil {
		p.onRead()
	}
	if p.total > 0 {
		if percent := int(p.read * 100 / p.total); percent >= p.step+5 {
			p.step = percent - percent%5
			fmt.Fprintf(os.Stderr, "\r  %3d%%  %s   ", p.step, humanBytes(p.read))
		}
	}
	return n, err
}

// done terminates the progress line. It only claims 100% when the whole body
// was actually received.
func (p *progressReader) done() {
	if p.total <= 0 {
		return
	}
	if p.read >= p.total {
		fmt.Fprintf(os.Stderr, "\r  100%%  %s   \n", humanBytes(p.read))
		return
	}
	fmt.Fprintln(os.Stderr)
}

func parseFlags() options {
	var opts options
	flags := flag.NewFlagSet("packmaafw", flag.ExitOnError)
	flags.StringVar(&opts.archive, "archive", "", "local path or URL of a MaaFramework release archive")
	flags.StringVar(&opts.version, "version", "", "MaaFramework release tag, e.g. v5.13.0 (default: read from "+versionFile+")")
	flags.StringVar(&opts.platform, "platform", defaultPlatform, "release platform to download")
	flags.StringVar(&opts.repo, "repo", defaultRepo, "GitHub repository publishing the release")
	flags.StringVar(&opts.assets, "assets", assetsDir, "directory holding release archives; an archive found here is used instead of downloading")
	flags.StringVar(&opts.out, "out", defaultOut, "directory receiving "+containerName+" and "+versionName)
	flags.Parse(os.Args[1:])
	return opts
}

func archiveName(platform, version string) string {
	return fmt.Sprintf("MAA-%s-%s.zip", platform, version)
}

func releaseURL(repo, platform, version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, version, archiveName(platform, version))
}

// normalizeVersion accepts both "5.13.0" and the tagged form "v5.13.0".
func normalizeVersion(version string) string {
	if version != "" && !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func isURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func githubToken() string {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token
		}
	}
	return ""
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
