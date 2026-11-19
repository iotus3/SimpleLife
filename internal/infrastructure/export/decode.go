package export

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Decode reads a file back into documents: one for CSV and Markdown, one per sheet for a
// workbook. It reads what this application writes and refuses to guess at anything else.
func Decode(layout export.Layout, data []byte) ([]export.Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, shared.Invalid("import.file.empty", "file", "the file is empty")
	}
	switch layout {
	case export.LayoutCSV:
		return decodeCSV(data)
	case export.LayoutMarkdown:
		return decodeMarkdown(data)
	case export.LayoutXLSX:
		return decodeXLSX(data)
	}
	return nil, shared.Invalid("import.file.layout", "file", "unsupported file format")
}

// decodeCSV reads a comma-separated table. Quotes and embedded newlines are the
// encoder's business; LazyQuotes keeps a hand-edited file readable.
func decodeCSV(data []byte) ([]export.Document, error) {
	reader := csv.NewReader(bytes.NewReader(stripBOM(data)))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = false
	records, err := reader.ReadAll()
	if err != nil {
		return nil, shared.Invalid("import.file.csv", "file", "the file is not readable as CSV").Wrap(err)
	}
	document, err := documentOf("", records)
	if err != nil {
		return nil, err
	}
	return []export.Document{document}, nil
}

// decodeMarkdown reads the first table of a markdown file: the header row, the
// separator row, then the data rows.
func decodeMarkdown(data []byte) ([]export.Document, error) {
	var records [][]string
	seenHeader, seenSeparator := false, false
	for _, line := range strings.Split(string(stripBOM(data)), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if !strings.HasPrefix(line, "|") {
			if seenHeader && seenSeparator && line == "" {
				break // the table ended
			}
			if seenHeader && !seenSeparator {
				return nil, shared.Invalid("import.file.markdown", "file", "the table has no separator row")
			}
			continue
		}
		cells := markdownCells(line)
		switch {
		case !seenHeader:
			seenHeader = true
			records = append(records, cells)
		case !seenSeparator:
			seenSeparator = true
		default:
			records = append(records, cells)
		}
	}
	if !seenHeader {
		return nil, shared.Invalid("import.file.markdown", "file", "the file holds no markdown table")
	}
	if !seenSeparator {
		return nil, shared.Invalid("import.file.markdown", "file", "the table has no separator row")
	}
	document, err := documentOf("", records)
	if err != nil {
		return nil, err
	}
	return []export.Document{document}, nil
}

// markdownCells splits one row on the pipes that are not escaped.
func markdownCells(line string) []string {
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var current strings.Builder
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '|':
			current.WriteByte('|')
			i++
		case line[i] == '|':
			cells = append(cells, markdownCell(current.String()))
			current.Reset()
		default:
			current.WriteByte(line[i])
		}
	}
	cells = append(cells, markdownCell(current.String()))
	return cells
}

// markdownCell trims a cell and turns the line breaks the exporter encoded as
// <br> back into real ones, so a multi-line note survives the round trip.
func markdownCell(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "<br/>", "\n")
	return strings.ReplaceAll(text, "<br>", "\n")
}

// documentOf trims trailing empty columns and rows, so a file that ends with the
// spreadsheet's leftover empty cells imports as the table the user sees.
func documentOf(name string, records [][]string) (export.Document, error) {
	if len(records) == 0 {
		return export.Document{}, shared.Invalid("import.file.empty", "file", "the file holds no table")
	}
	width := 0
	for _, record := range records {
		if n := lastFilled(record) + 1; n > width {
			width = n
		}
	}
	if width == 0 {
		return export.Document{}, shared.Invalid("import.file.empty", "file", "the table is empty")
	}
	rows := make([][]string, 0, len(records)-1)
	for _, record := range records[1:] {
		if lastFilled(record) < 0 {
			continue
		}
		row := make([]string, width)
		for i := 0; i < width && i < len(record); i++ {
			row[i] = record[i]
		}
		rows = append(rows, row)
	}
	headers := make([]string, width)
	copy(headers, records[0])
	return export.Document{Name: name, Headers: headers, Rows: rows}, nil
}

// lastFilled is the index of the last non-blank cell, or -1.
func lastFilled(cells []string) int {
	for i := len(cells) - 1; i >= 0; i-- {
		if strings.TrimSpace(cells[i]) != "" {
			return i
		}
	}
	return -1
}

// stripBOM removes the byte-order mark a spreadsheet may put in front of the first
// header, which would otherwise become part of the first column's name.
func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
}

// ── xlsx ───────────────────────────────────────────────────────────────

// readWorkbookXML is the sheet list of a workbook.
type readWorkbookXML struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

// readRelationsXML maps the sheet ids of the workbook to their part in the zip.
type readRelationsXML struct {
	Relations []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

// readSharedStringsXML is the string table every text cell points into.
type readSharedStringsXML struct {
	Items []struct {
		Text string   `xml:"t"`
		Runs []string `xml:"r>t"`
	} `xml:"si"`
}

// readSheetXML is one worksheet: rows of cells, each cell either inline text, a
// shared string index or a literal value.
type readSheetXML struct {
	Rows []readRowXML `xml:"sheetData>row"`
}

// readRowXML is one row. The column of a cell comes from its own reference, not
// from its position, because a spreadsheet omits empty cells entirely.
type readRowXML struct {
	Cells []readCellXML `xml:"c"`
}

// readCellXML is one cell.
type readCellXML struct {
	Ref  string `xml:"r,attr"`
	Type string `xml:"t,attr"`
	V    string `xml:"v"`
	IS   struct {
		Text string   `xml:"t"`
		Runs []string `xml:"r>t"`
	} `xml:"is"`
}

// decodeXLSX reads every sheet of a workbook this application wrote. It is a small
// reader on purpose: the export writes plain cells without styles, formulas or
// merged ranges, and a full spreadsheet engine would be far more than that needs.
func decodeXLSX(data []byte) ([]export.Document, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, shared.Invalid("import.file.xlsx", "file", "the file is not a workbook").Wrap(err)
	}
	parts := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		parts[file.Name] = file
	}
	if parts["xl/workbook.xml"] == nil {
		return nil, shared.Invalid("import.file.xlsx", "file", "the workbook has no sheet list")
	}

	var book readWorkbookXML
	if err := readXML(parts["xl/workbook.xml"], &book); err != nil {
		return nil, err
	}
	var relations readRelationsXML
	if parts["xl/_rels/workbook.xml.rels"] != nil {
		if err := readXML(parts["xl/_rels/workbook.xml.rels"], &relations); err != nil {
			return nil, err
		}
	}
	targets := make(map[string]string, len(relations.Relations))
	for _, relation := range relations.Relations {
		target := strings.TrimPrefix(relation.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		targets[relation.ID] = target
	}

	// The string table a cell may point into. This application writes inline
	// strings, but a workbook edited in Excel carries a table instead, so both are
	// read.
	var pool []string
	if parts["xl/sharedStrings.xml"] != nil {
		var table readSharedStringsXML
		if err := readXML(parts["xl/sharedStrings.xml"], &table); err != nil {
			return nil, err
		}
		pool = make([]string, 0, len(table.Items))
		for _, item := range table.Items {
			pool = append(pool, item.Text+strings.Join(item.Runs, ""))
		}
	}

	var documents []export.Document
	for index, sheet := range book.Sheets {
		part := targets[sheet.RID]
		if part == "" {
			// A workbook without relationships numbers its parts by position.
			part = "xl/worksheets/sheet" + strconv.Itoa(index+1) + ".xml"
		}
		file := parts[part]
		if file == nil {
			continue
		}
		var body readSheetXML
		if err := readXML(file, &body); err != nil {
			return nil, err
		}
		records := make([][]string, 0, len(body.Rows))
		for _, row := range body.Rows {
			cells := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				column := columnIndex(cell.Ref)
				for len(cells) < column {
					cells = append(cells, "")
				}
				cells = append(cells, cellValue(cell.Type, cell.V, cell.IS.Text+strings.Join(cell.IS.Runs, ""), pool))
			}
			records = append(records, cells)
		}
		if len(records) == 0 {
			continue
		}
		document, err := documentOf(sheet.Name, records)
		if err != nil {
			// An empty sheet is normal in a workbook: it is skipped, not reported.
			if shared.IsKind(err, shared.KindInvalid) {
				continue
			}
			return nil, err
		}
		documents = append(documents, document)
	}
	if len(documents) == 0 {
		return nil, shared.Invalid("import.file.xlsx", "file", "the workbook holds no table")
	}
	return documents, nil
}

func readXML(file *zip.File, target any) error {
	reader, err := file.Open()
	if err != nil {
		return shared.Invalid("import.file.xlsx", "file", "the workbook part is unreadable").Wrap(err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(io.LimitReader(reader, 16<<20))
	if err != nil {
		return shared.Invalid("import.file.xlsx", "file", "the workbook part is unreadable").Wrap(err)
	}
	if err := xml.Unmarshal(body, target); err != nil {
		return shared.Invalid("import.file.xlsx", "file", "the workbook part is not valid XML").Wrap(err)
	}
	return nil
}

// cellValue turns one cell into the text the table holds. Numbers keep their
// literal spelling, which is what the export wrote.
func cellValue(kind, value, inline string, pool []string) string {
	switch kind {
	case "s":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 || n >= len(pool) {
			return ""
		}
		return pool[n]
	case "inlineStr":
		return inline
	case "str":
		return value
	case "b":
		if strings.TrimSpace(value) == "1" {
			return "true"
		}
		return "false"
	}
	return value
}

// columnIndex turns "B12" into 1. A reference without letters is column 0.
func columnIndex(ref string) int {
	column := 0
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c < 'A' || c > 'Z' {
			break
		}
		column = column*26 + int(c-'A'+1)
	}
	if column == 0 {
		return 0
	}
	return column - 1
}
