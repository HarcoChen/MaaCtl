package tests

import (
	"os"
	"path/filepath"
	"testing"

	"maactl/internal/pi"
)

// sampleInterface returns the path of MaaFramework's reference
// ProjectInterface. The whole maafw/ tree is gitignored, so a fresh checkout
// has no sample: the tests that need it skip instead of failing `go test ./...`.
func sampleInterface(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "maafw", "sample", "interface.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("MaaFramework sample interface is not present: %v", err)
	}
	return path
}

// writeProject writes body to a fresh temporary interface.json and returns its
// path. It is the single place the tests turn a JSON fixture into a file.
func writeProject(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, path, body)
	return path
}

// loadProject writes body to a fresh interface.json and loads it, returning the
// file path together with the parsed project so callers take what they need.
func loadProject(t *testing.T, body string) (string, *pi.Loaded) {
	t.Helper()
	path := writeProject(t, body)
	project, err := pi.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return path, project
}
