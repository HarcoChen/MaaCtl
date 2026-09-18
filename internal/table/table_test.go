package table

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrintDropsCellsBeyondTheHeader guards a malformed row: with more cells
// than the header has columns there is no column width to align them to, so the
// extra cells are dropped instead of indexing past the widths slice.
func TestPrintDropsCellsBeyondTheHeader(t *testing.T) {
	var out bytes.Buffer
	if err := Print(&out, []string{"name", "value"}, [][]string{{"a", "b", "extra"}}); err != nil {
		t.Fatal(err)
	}
	if want := "name  value\n----  -----\na     b\n"; out.String() != want {
		t.Errorf("Print = %q, want %q", out.String(), want)
	}
}

// TestPrintKeepsShortRowsUnchanged pins the rows that already fitted the header.
func TestPrintKeepsShortRowsUnchanged(t *testing.T) {
	var out bytes.Buffer
	if err := Print(&out, []string{"name", "value"}, [][]string{{"only"}}); err != nil {
		t.Fatal(err)
	}
	if want := "name  value\n----  -----\nonly\n"; out.String() != want {
		t.Errorf("Print = %q, want %q", out.String(), want)
	}
}

// TestPrintKeepsCallerRowsUntouched pins that Print copies the rows it formats
// instead of rewriting the caller's slice.
func TestPrintKeepsCallerRowsUntouched(t *testing.T) {
	rows := [][]string{{"a\tb"}}
	var out bytes.Buffer
	if err := Print(&out, []string{"name"}, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0][0] != "a\tb" {
		t.Errorf("Print modified the caller's row: %q", rows[0][0])
	}
	if !strings.Contains(out.String(), `a\tb`) {
		t.Errorf("the escaped value should still be written:\n%s", out.String())
	}
}

// TestPrintMeasuresColorsAsText pins that ANSI color sequences do not count
// towards a cell's width, so colored cells stay aligned instead of pushing the
// following column out of place.
func TestPrintMeasuresColorsAsText(t *testing.T) {
	const red = "\x1b[31mred\x1b[0m"
	var out bytes.Buffer
	if err := Print(&out, []string{"name", "value"}, [][]string{{red, "x"}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out.String(), "\n")
	if len(lines) < 3 {
		t.Fatalf("unexpected output:\n%q", out.String())
	}
	// "red" is three columns wide, so the second column starts after the three
	// spaces that align it under "name" and the two column gaps.
	if want := red + "   x"; lines[2] != want {
		t.Errorf("row = %q, want %q", lines[2], want)
	}
}
