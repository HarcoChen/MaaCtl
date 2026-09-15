package cli

import (
	"io"

	"maactl/internal/output"
	"maactl/internal/table"
)

// emit writes rows as JSON under --json, and as the aligned table described by
// header and cells otherwise. It is the single place a query decides between its
// machine-readable and its human-readable form.
func emit(out io.Writer, jsonMode bool, rows any, header []string, cells [][]string) error {
	if jsonMode {
		return output.JSON(out, rows)
	}
	return table.Print(out, header, cells)
}

// emitLines writes payload as JSON under --json, and the lines text renders
// otherwise. Queries whose text form is not a table (one record per line, or a
// hand-formatted summary) use it; the JSON payload is theirs to choose.
func emitLines(out io.Writer, jsonMode bool, payload any, text func(io.Writer) error) error {
	if jsonMode {
		return output.JSON(out, payload)
	}
	return text(out)
}
