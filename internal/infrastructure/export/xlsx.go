package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/export"
)

// encodeXLSX writes a minimal, style-free workbook with inline strings: no shared table and no
// second pass, at the cost of a slightly larger file.
func encodeXLSX(sheets []export.Sheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("export: nothing to encode")
	}
	names := sheetNames(sheets)
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)

	entries := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypes(len(sheets))},
		{"_rels/.rels", rootRels()},
		{"xl/workbook.xml", workbookXML(names)},
		{"xl/_rels/workbook.xml.rels", workbookRels(len(sheets))},
	}
	for _, entry := range entries {
		if err := writeZip(archive, entry.name, entry.body); err != nil {
			_ = archive.Close()
			return nil, err
		}
	}
	for i, sheet := range sheets {
		body, err := sheetXML(sheet)
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		if err := writeZip(archive, fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), body); err != nil {
			_ = archive.Close()
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("export: xlsx close: %w", err)
	}
	return buf.Bytes(), nil
}

func writeZip(archive *zip.Writer, name, body string) error {
	writer, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("export: xlsx entry %s: %w", name, err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		return fmt.Errorf("export: xlsx write %s: %w", name, err)
	}
	return nil
}

// sheetXML renders one worksheet.
func sheetXML(sheet export.Sheet) (string, error) {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<sheetData>`)
	row := 1
	b.WriteString(`<row r="1">`)
	for col, header := range sheet.Headers {
		b.WriteString(cellXML(col, row, header))
	}
	b.WriteString(`</row>`)
	for _, values := range sheet.Rows {
		row++
		b.WriteString(`<row r="` + strconv.Itoa(row) + `">`)
		for col, value := range values {
			b.WriteString(cellXML(col, row, value))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String(), nil
}

// cellXML writes one inline-string cell. styleIndex 0 keeps the default format, so
// Excel guesses per value — which is what a plain export wants.
func cellXML(col, row int, value string) string {
	ref := columnName(col) + strconv.Itoa(row)
	text := xmlEscape(value)
	if text == "" {
		return `<c r="` + ref + `"/>`
	}
	return `<c r="` + ref + `" t="inlineStr"><is><t xml:space="preserve">` + text + `</t></is></c>`
}

// columnName turns a zero-based column index into A, B, ... Z, AA, AB, ...
func columnName(index int) string {
	var out []byte
	for index >= 0 {
		out = append([]byte{byte('A' + index%26)}, out...)
		index = index/26 - 1
	}
	return string(out)
}

// sheetNames makes the sheet titles legal and unique: Excel refuses a name longer
// than 31 characters, containing []:*?/\ , or duplicated inside the workbook.
func sheetNames(sheets []export.Sheet) []string {
	used := make(map[string]bool, len(sheets))
	out := make([]string, len(sheets))
	for i, sheet := range sheets {
		name := sanitizeSheetName(sheet.Name)
		if name == "" {
			name = sanitizeSheetName(sheet.FileBase)
		}
		if name == "" {
			name = "Sheet" + strconv.Itoa(i+1)
		}
		base := name
		for n := 2; used[strings.ToLower(name)]; n++ {
			suffix := " (" + strconv.Itoa(n) + ")"
			trimmed := base
			if len(trimmed)+len(suffix) > maxSheetNameLen {
				trimmed = trimmed[:maxSheetNameLen-len(suffix)]
			}
			name = trimmed + suffix
		}
		used[strings.ToLower(name)] = true
		out[i] = name
	}
	return out
}

const maxSheetNameLen = 31

func sanitizeSheetName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '[', ']', ':', '*', '?', '/', '\\':
			b.WriteRune(' ')
		default:
			if r < 0x20 {
				continue
			}
			b.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(b.String())
	// Excel also counts UTF-16 code units; taking runes is close enough and errs on
	// the short side, which Excel accepts.
	runes := []rune(clean)
	if len(runes) > maxSheetNameLen {
		runes = runes[:maxSheetNameLen]
	}
	return strings.TrimSpace(string(runes))
}
