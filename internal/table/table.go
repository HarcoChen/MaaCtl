// Package table renders aligned plain-text tables using terminal cell widths.
package table

import (
	"io"
	"strings"

	"maactl/internal/output"

	"github.com/mattn/go-runewidth"
)

// Print writes a header, a separator, and one line per row. Values are kept on
// a single line and padded according to their terminal display width so
// Chinese names align with Latin text and digits. Empty values render as "-",
// ANSI color sequences are not counted as width, cells beyond the last header
// are dropped, the caller's rows are never modified, and an empty row set
// renders a placeholder.
func Print(out io.Writer, headers []string, rows [][]string) error {
	// Use terminal cell widths so Chinese names align with Latin text and digits.
	width := runewidth.NewCondition()
	width.EastAsianWidth = false
	widths := make([]int, len(headers))
	escape := strings.NewReplacer("\t", "\\t", "\r", "\\r", "\n", "\\n")
	for i, header := range headers {
		widths[i] = width.StringWidth(stripANSI(header))
	}
	lines := make([][]string, 0, len(rows))
	for _, row := range rows {
		// The row is copied instead of rewritten in place, so Print never changes
		// the slice the caller passed in.
		line := make([]string, 0, min(len(row), len(widths)))
		for i, cell := range row {
			if i >= len(widths) {
				// A row with more cells than the header has columns has nowhere to
				// put them; dropping them keeps the column widths meaningful and
				// keeps widths[i] in range.
				break
			}
			// Keep each value on one line, including labels containing tabs/newlines.
			text := output.Value(escape.Replace(cell))
			widths[i] = max(widths[i], width.StringWidth(stripANSI(text)))
			line = append(line, text)
		}
		lines = append(lines, line)
	}
	var result strings.Builder
	writeRow := func(row []string) {
		for i, cell := range row {
			result.WriteString(cell)
			if i < len(row)-1 {
				result.WriteString(strings.Repeat(" ", widths[i]-width.StringWidth(stripANSI(cell))+2))
			}
		}
		result.WriteByte('\n')
	}
	writeRow(headers)
	separator := make([]string, len(headers))
	for i, size := range widths {
		separator[i] = strings.Repeat("-", size)
	}
	writeRow(separator)
	for _, row := range lines {
		writeRow(row)
	}
	if len(rows) == 0 {
		result.WriteString("（无数据）\n")
	}
	_, err := io.WriteString(out, result.String())
	return err
}

// stripANSI removes CSI escape sequences so a colored cell is measured by what
// the terminal prints instead of by the bytes carrying the color. The original
// text is what gets written; only the measurement uses the stripped form.
func stripANSI(value string) string {
	if !strings.Contains(value, "\x1b[") {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] != 0x1b || i+1 >= len(value) || value[i+1] != '[' {
			b.WriteByte(value[i])
			i++
			continue
		}
		// A CSI sequence ends at the first byte in @..~; parameter and
		// intermediate bytes all come before it.
		end := i + 2
		for end < len(value) && (value[end] < 0x40 || value[end] > 0x7e) {
			end++
		}
		if end >= len(value) {
			// An unterminated sequence: nothing after it can be measured.
			break
		}
		i = end + 1
	}
	return b.String()
}
