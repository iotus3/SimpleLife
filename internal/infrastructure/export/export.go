// Package export encodes the materialised sheets into a file: the formatting decision was
// made upstream, so these are pure functions of []export.Sheet.
package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/export"
)

// Encode writes every sheet in the requested layout. XLSX holds them in one workbook; CSV and
// Markdown hold one table per file, so several tables leave as a zip.
func Encode(layout export.Layout, sheets []export.Sheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("export: nothing to encode")
	}
	switch layout {
	case export.LayoutCSV:
		if len(sheets) == 1 {
			return encodeCSV(sheets[0])
		}
		return zipSheets(sheets, ".csv", encodeCSV)
	case export.LayoutMarkdown:
		if len(sheets) == 1 {
			return encodeMarkdown(sheets[0])
		}
		return zipSheets(sheets, ".md", encodeMarkdown)
	case export.LayoutXLSX:
		return encodeXLSX(sheets)
	}
	return nil, fmt.Errorf("export: unsupported layout %v", layout)
}

// zipSheets packs one file per sheet, plus a README describing the contents.
func zipSheets(
	sheets []export.Sheet,
	ext string,
	encode func(export.Sheet) ([]byte, error),
) ([]byte, error) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	used := make(map[string]bool, len(sheets))
	for _, sheet := range sheets {
		data, err := encode(sheet)
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		name := uniqueName(sheet.FileBase, ext, used)
		writer, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("export: zip entry %s: %w", name, err)
		}
		if _, err := writer.Write(data); err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("export: zip write %s: %w", name, err)
		}
	}
	readme, err := archive.Create("README.txt")
	if err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("export: zip readme: %w", err)
	}
	if _, err := readme.Write([]byte(readmeText(sheets, ext, used))); err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("export: zip readme: %w", err)
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("export: zip close: %w", err)
	}
	return buf.Bytes(), nil
}

// uniqueName keeps two tables from overwriting each other in the archive. The base
// names are distinct today, but a future table that reuses one would otherwise
// silently lose a file.
func uniqueName(base, ext string, used map[string]bool) string {
	if base == "" {
		base = "sheet"
	}
	name := base + ext
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	used[name] = true
	return name
}

// readmeText is deliberately ASCII-only: it travels inside a zip that may be opened
// on a machine whose locale cannot read the sheet titles.
func readmeText(sheets []export.Sheet, ext string, used map[string]bool) string {
	var b strings.Builder
	b.WriteString("SimpleLife export\n")
	b.WriteString("================\n\n")
	b.WriteString("Files in this archive (one per exported table):\n")
	for _, sheet := range sheets {
		fmt.Fprintf(&b, "  %-24s %d columns, %d rows\n", sheet.FileBase+ext, len(sheet.Headers), len(sheet.Rows))
	}
	b.WriteString("\nNotes:\n")
	b.WriteString("  * Headers follow the interface language; values do not.\n")
	b.WriteString("  * Dates are YYYY-MM-DD or YYYY-MM-DD HH:mm; amounts are decimal numbers\n")
	b.WriteString("    without a currency symbol, and the currency is its own column.\n")
	b.WriteString("  * Enum values are stable English keys (todo, expense, ...), so a script\n")
	b.WriteString("    does not have to know which language the app was set to.\n")
	return b.String()
}
