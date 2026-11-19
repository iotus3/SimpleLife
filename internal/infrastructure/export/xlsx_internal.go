package export

import (
	"strconv"
	"strings"
)

// The OOXML package skeleton. Only the parts Excel needs to open a workbook with
// text cells: content types, one relationship file for the package, the workbook,
// its relationship file, and the worksheets themselves.

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

func contentTypes(sheetCount int) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for i := 1; i <= sheetCount; i++ {
		b.WriteString(`<Override PartName="/xl/worksheets/sheet` + strconv.Itoa(i) +
			`.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func rootRels() string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`
}

func workbookXML(names []string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" `)
	b.WriteString(`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	b.WriteString(`<sheets>`)
	for i, name := range names {
		b.WriteString(`<sheet name="` + xmlEscape(name) + `" sheetId="` + strconv.Itoa(i+1) +
			`" r:id="rId` + strconv.Itoa(i+1) + `"/>`)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func workbookRels(sheetCount int) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= sheetCount; i++ {
		b.WriteString(`<Relationship Id="rId` + strconv.Itoa(i) +
			`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" ` +
			`Target="worksheets/sheet` + strconv.Itoa(i) + `.xml"/>`)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// xmlEscape escapes the five XML entities and drops the control characters XML 1.0
// forbids outright. A note pasted from another program can contain them, and one
// such byte makes the whole workbook unreadable.
func xmlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n', '\r':
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
