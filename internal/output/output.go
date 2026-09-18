// Package output provides shared text and JSON formatting helpers for the CLI.
package output

import (
	"encoding/json"
	"io"
	"strings"
)

// Value returns "-" for blank values so tables and summaries stay aligned.
func Value(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

// JSON writes v to w as indented JSON followed by a newline.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
