package export_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	encoder "github.com/iotus3/simplelife/app/internal/infrastructure/export"
)

// update rewrites the golden files instead of comparing against them, so a change to the
// output is a visible diff in review: go test ./internal/infrastructure/export/ -update
var update = flag.Bool("update", false, "rewrite the golden files")

// sampleSheets is the fixture the goldens are built from. It deliberately includes
// the cases that break naive encoders: Chinese text, a pipe, a comma, a quote, a
// newline, an empty cell and a negative amount.
func sampleSheets() []export.Sheet {
	return []export.Sheet{
		{
			Name:     "待办",
			FileBase: "tasks",
			Headers:  []string{"ID", "标题", "状态", "截止日", "标签"},
			Rows: [][]string{
				{"0001", "写方案", "todo", "2026-11-17", "work,urgent"},
				{"0002", `引用"引号"的标题`, "done", "2026-11-16", ""},
				{"0003", "含 | 竖线与\n换行的备注", "doing", "", "home"},
			},
		},
		{
			Name:     "收支流水",
			FileBase: "entries",
			Headers:  []string{"日期", "类型", "金额", "币种", "账户", "备注"},
			Rows: [][]string{
				{"2026-11-17 12:00", "expense", "-35.50", "CNY", "现金", "午饭"},
				{"2026-11-17 09:00", "income", "5000.00", "CNY", "银行卡", "工资"},
				{"2026-11-17 10:00", "transfer", "-200.00", "USD", "美元账户", "换汇"},
			},
		},
	}
}

// TestGoldenCSV pins the byte-exact CSV output, including the BOM and the CRLF
// line endings that Excel needs.
func TestGoldenCSV(t *testing.T) {
	for _, sheet := range sampleSheets() {
		data, err := encoder.Encode(export.LayoutCSV, []export.Sheet{sheet})
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		golden(t, sheet.FileBase+".csv", data)
		if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Fatalf("%s: the export must start with a UTF-8 BOM", sheet.FileBase)
		}
		if !bytes.Contains(data, []byte("\r\n")) {
			t.Fatalf("%s: rows must end with CRLF", sheet.FileBase)
		}
	}
}

// TestGoldenMarkdown pins the Markdown table output.
func TestGoldenMarkdown(t *testing.T) {
	for _, sheet := range sampleSheets() {
		data, err := encoder.Encode(export.LayoutMarkdown, []export.Sheet{sheet})
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		golden(t, sheet.FileBase+".md", data)
	}
}

// TestGoldenXLSXParts pins the workbook XML parts. The zip container itself has
// timestamps, so the comparison is on the unzipped parts, which is what a reader
// actually parses.
func TestGoldenXLSXParts(t *testing.T) {
	data, err := encoder.Encode(export.LayoutXLSX, sampleSheets())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := unzip(t, data)
	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/worksheets/sheet1.xml",
		"xl/worksheets/sheet2.xml",
	} {
		body, ok := parts[name]
		if !ok {
			t.Fatalf("the workbook is missing %s (have %v)", name, keysOf(parts))
		}
		golden(t, filepath.Join("xlsx", strings.ReplaceAll(name, "/", "_")), []byte(body))
		// A well-formed XML document is the minimum a reader will accept, and the
		// decoder catches a mis-escaped value that a string comparison would not.
		decoder := xml.NewDecoder(strings.NewReader(body))
		for {
			_, err := decoder.Token()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("%s is not well-formed XML: %v", name, err)
			}
		}
	}
}

func TestCSVQuotesRFC4180(t *testing.T) {
	// The csv package does the quoting; this test pins that we did not disable it.
	data, err := encoder.Encode(export.LayoutCSV, []export.Sheet{{
		Name: "t", FileBase: "t", Headers: []string{"a"}, Rows: [][]string{{`x,"y"`}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.Contains(body, `"x,""y"""`) {
		t.Fatalf("a comma and a quote must be escaped, got %q", body)
	}
}

func TestMarkdownEscapesTableCellBreakers(t *testing.T) {
	data, err := encoder.Encode(export.LayoutMarkdown, []export.Sheet{{
		Name:     "表",
		FileBase: "t",
		Headers:  []string{"a", "b"},
		Rows:     [][]string{{"x|y", "line1\nline2"}, {"", "   "}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `x\|y`) {
		t.Fatalf("a pipe must be escaped: %q", body)
	}
	if !strings.Contains(body, "line1<br>line2") {
		t.Fatalf("a newline must become <br>: %q", body)
	}
	if strings.Count(body, "\n") != 5 {
		// heading, blank line, header row, separator, two data rows = 6 lines.
		if strings.Count(body, "\n") != 6 {
			t.Fatalf("unexpected line count: %q", body)
		}
	}
	if !strings.Contains(body, "—") {
		t.Fatalf("an empty cell must show an em dash: %q", body)
	}
}

func TestMarkdownSaysWhenTheRangeIsEmpty(t *testing.T) {
	data, err := encoder.Encode(export.LayoutMarkdown, []export.Sheet{{
		Name: "空表", FileBase: "empty", Headers: []string{"a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "No rows") {
		t.Fatalf("an empty table must say so: %q", data)
	}
}

func TestMultiTableCSVBecomesAZip(t *testing.T) {
	data, err := encoder.Encode(export.LayoutCSV, sampleSheets())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := unzip(t, data)
	if len(parts) != 3 {
		t.Fatalf("want two tables plus a README, got %v", keysOf(parts))
	}
	for _, name := range []string{"tasks.csv", "entries.csv", "README.txt"} {
		if _, ok := parts[name]; !ok {
			t.Fatalf("missing %s in %v", name, keysOf(parts))
		}
	}
	if !strings.Contains(parts["README.txt"], "entries.csv") {
		t.Fatalf("the README must list the tables: %q", parts["README.txt"])
	}
	// Each entry keeps its BOM: it is opened on its own, outside the archive.
	if !strings.HasPrefix(parts["tasks.csv"], "\ufeff") {
		t.Fatal("the CSV inside the zip lost its BOM")
	}
}

func TestMultiTableMarkdownBecomesAZip(t *testing.T) {
	data, err := encoder.Encode(export.LayoutMarkdown, sampleSheets())
	if err != nil {
		t.Fatal(err)
	}
	parts := unzip(t, data)
	for _, name := range []string{"tasks.md", "entries.md", "README.txt"} {
		if _, ok := parts[name]; !ok {
			t.Fatalf("missing %s in %v", name, keysOf(parts))
		}
	}
}

func TestXLSXSheetNamesAreLegalAndUnique(t *testing.T) {
	sheets := []export.Sheet{
		{Name: "很长的中文表名超过了三十一个字符的限制需要被截断处理", FileBase: "a", Headers: []string{"h"}},
		{Name: "重复[的]名字:带*非法?字符/反斜杠\\", FileBase: "b", Headers: []string{"h"}},
		{Name: "重复[的]名字:带*非法?字符/反斜杠\\", FileBase: "c", Headers: []string{"h"}},
		{Name: "", FileBase: "fallback", Headers: []string{"h"}},
	}
	data, err := encoder.Encode(export.LayoutXLSX, sheets)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	workbook := unzip(t, data)["xl/workbook.xml"]
	names := sheetNameAttributes(t, workbook)
	if len(names) != len(sheets) {
		t.Fatalf("sheet count = %d, want %d", len(names), len(sheets))
	}
	seen := map[string]bool{}
	for _, name := range names {
		if len([]rune(name)) > 31 {
			t.Fatalf("sheet name %q is longer than 31 characters", name)
		}
		for _, bad := range []string{"[", "]", ":", "*", "?", "/", "\\"} {
			if strings.Contains(name, bad) {
				t.Fatalf("sheet name %q contains %q", name, bad)
			}
		}
		if seen[strings.ToLower(name)] {
			t.Fatalf("duplicate sheet name %q", name)
		}
		seen[strings.ToLower(name)] = true
	}
	if names[3] != "fallback" {
		t.Fatalf("an unnamed sheet must fall back to the file base, got %q", names[3])
	}
}

func TestXLSXEscapesXMLAndDropsControlCharacters(t *testing.T) {
	data, err := encoder.Encode(export.LayoutXLSX, []export.Sheet{{
		Name: "t", FileBase: "t",
		Headers: []string{"h"},
		Rows:    [][]string{{`a<b & c>"d"`, "tab\there\x01bell"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := unzip(t, data)["xl/worksheets/sheet1.xml"]
	for _, want := range []string{"a&lt;b &amp; c&gt;&quot;d&quot;", "tab\therebell"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %q", want, body)
		}
	}
	if strings.ContainsAny(body, "\x01") {
		t.Fatal("a control character must be dropped, not written")
	}
}

func TestXLSXCellReferences(t *testing.T) {
	// 28 columns crosses the Z/AA boundary, where an off-by-one shows up.
	headers := make([]string, 28)
	row := make([]string, 28)
	for i := range headers {
		headers[i] = "h"
		row[i] = "v"
	}
	data, err := encoder.Encode(export.LayoutXLSX, []export.Sheet{{
		Name: "t", FileBase: "t", Headers: headers, Rows: [][]string{row},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := unzip(t, data)["xl/worksheets/sheet1.xml"]
	for _, ref := range []string{`r="A1"`, `r="Z1"`, `r="AA1"`, `r="AB1"`, `r="A2"`, `r="AB2"`} {
		if !strings.Contains(body, ref) {
			t.Fatalf("missing cell %s in the sheet", ref)
		}
	}
}

func TestEncodeRejectsAnEmptyPlan(t *testing.T) {
	if _, err := encoder.Encode(export.LayoutCSV, nil); err == nil {
		t.Fatal("encoding nothing must fail rather than write an empty file")
	}
	if _, err := encoder.Encode(export.Layout(99), sampleSheets()); err == nil {
		t.Fatal("an unknown layout must fail")
	}
}

// ── helpers ────────────────────────────────────────────────────────────

// golden compares data with testdata/name, or writes it when -update is passed.
func golden(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run go test -update to create it)", name, err)
	}
	if !bytes.Equal(want, data) {
		t.Fatalf("%s does not match the golden file (run go test -update after checking the diff)\n--- got ---\n%s\n--- want ---\n%s",
			name, data, want)
	}
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the output is not a readable zip: %v", err)
	}
	out := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(handle); err != nil {
			_ = handle.Close()
			t.Fatalf("read %s: %v", file.Name, err)
		}
		_ = handle.Close()
		out[file.Name] = buf.String()
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// sheetNameAttributes pulls the name attribute out of every <sheet> element.
func sheetNameAttributes(t *testing.T, workbook string) []string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(workbook))
	var out []string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == "name" {
				out = append(out, attr.Value)
			}
		}
	}
	return out
}
