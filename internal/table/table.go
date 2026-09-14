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
// and an empty row set renders a placeholder.
func Print(out io.Writer, headers []string, rows [][]string) error {
	// Use terminal cell widths so Chinese names align with Latin text and digits.
	width := runewidth.NewCondition()
	width.EastAsianWidth = false
	widths := make([]int, len(headers))
	escape := strings.NewReplacer("\t", "\\t", "\r", "\\r", "\n", "\\n")
	for i, header := range headers {
		widths[i] = width.StringWidth(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			// Keep each value on one line, including labels containing tabs/newlines.
			row[i] = output.Value(escape.Replace(cell))
			widths[i] = max(widths[i], width.StringWidth(row[i]))
		}
	}
	var result strings.Builder
	writeRow := func(row []string) {
		for i, cell := range row {
			result.WriteString(cell)
			if i < len(row)-1 {
				result.WriteString(strings.Repeat(" ", widths[i]-width.StringWidth(cell)+2))
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
	for _, row := range rows {
		writeRow(row)
	}
	if len(rows) == 0 {
		result.WriteString("（无数据）\n")
	}
	_, err := io.WriteString(out, result.String())
	return err
}
