package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePI writes a PI file (creating parent directories) and returns its path.
func writePI(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadRejectsCyclicImport covers the import walk's cycle guard.
func TestLoadRejectsCyclicImport(t *testing.T) {
	dir := t.TempDir()
	writePI(t, dir, "a.json", `{"import": ["b.json"]}`)
	writePI(t, dir, "b.json", `{"import": ["a.json"]}`)
	main := writePI(t, dir, "interface.json", `{"interface_version": 2, "import": ["a.json"]}`)

	_, err := Load(main)
	if err == nil || !strings.Contains(err.Error(), "cyclic PI import") {
		t.Fatalf("err = %v, want a cyclic import error", err)
	}
}

// TestLoadRejectsMissingImport covers an import pointing at a file that is not
// there.
func TestLoadRejectsMissingImport(t *testing.T) {
	dir := t.TempDir()
	main := writePI(t, dir, "interface.json", `{"interface_version": 2, "import": ["missing.json"]}`)

	_, err := Load(main)
	if err == nil || !strings.Contains(err.Error(), "missing.json") {
		t.Fatalf("err = %v, want a read error naming the missing import", err)
	}
}

// TestNegotiateLanguage covers the case-insensitive hit and the fallback chain.
func TestNegotiateLanguage(t *testing.T) {
	available := map[string]string{"zh_cn": "zh.json", "en_us": "en.json", "de_de": "de.json"}
	for _, tc := range []struct {
		requested string
		want      string
	}{
		{"zh_cn", "zh_cn"}, // exact
		{"ZH_CN", "zh_cn"}, // case-insensitive
		{"zh", "zh_cn"},    // primary subtag
		{"EN", "en_us"},    // primary subtag, case-insensitive
		{"fr", "zh_cn"},    // unknown falls back to zh_cn
		{"", "zh_cn"},      // empty request prefers zh_cn
	} {
		if got := NegotiateLanguage(tc.requested, available); got != tc.want {
			t.Errorf("NegotiateLanguage(%q) = %q, want %q", tc.requested, got, tc.want)
		}
	}
	if got := NegotiateLanguage("", map[string]string{"en_us": "en", "de_de": "de"}); got != "en_us" {
		t.Errorf("fallback with en_us = %q, want en_us", got)
	}
	if got := NegotiateLanguage("", map[string]string{"it_it": "it", "de_de": "de"}); got != "de_de" {
		t.Errorf("fallback without a preferred code = %q, want the first sorted code", got)
	}
	if got := NegotiateLanguage("fr", nil); got != "fr" {
		t.Errorf("a project without languages = %q, want the request", got)
	}
}
