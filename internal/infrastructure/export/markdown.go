package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/export"
)

// encodeMarkdown writes one sheet as a GitHub-flavoured table; pipes and newlines are the only
// things that need escaping.
func encodeMarkdown(sheet export.Sheet) ([]byte, error) {
	var buf bytes.Buffer
	title := strings.TrimSpace(sheet.Name)
	if title == "" {
		title = sheet.FileBase
	}
	buf.WriteString("## " + escapeInline(title) + "\n\n")
	if len(sheet.Headers) == 0 {
		return nil, fmt.Errorf("export: markdown sheet %q has no columns", sheet.FileBase)
	}
	buf.WriteString("| " + markdownRow(sheet.Headers) + " |\n")
	align := make([]string, len(sheet.Headers))
	for i := range align {
		align[i] = "---"
	}
	buf.WriteString("| " + strings.Join(align, " | ") + " |\n")
	for _, row := range sheet.Rows {
		buf.WriteString("| " + markdownRow(row) + " |\n")
	}
	if len(sheet.Rows) == 0 {
		// An empty table is legitimate (no data in range); saying so beats a table
		// that looks truncated.
		buf.WriteString("\n_No rows in the selected range._\n")
	}
	return buf.Bytes(), nil
}

func markdownRow(cells []string) string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = escapeCell(cell)
	}
	return strings.Join(out, " | ")
}

// escapeCell makes one value safe in a table cell: pipes are escaped, line breaks become <br>,
// and an empty value becomes an em dash.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	s = strings.ReplaceAll(s, "\r", "<br>")
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// escapeInline keeps a heading on one line.
func escapeInline(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
