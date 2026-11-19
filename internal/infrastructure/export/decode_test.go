package export_test

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	infra "github.com/iotus3/simplelife/app/internal/infrastructure/export"
)

// trickySheet holds the values that break a naive reader: a comma, a pipe, a
// newline, a quote and text that is not ASCII.
func trickySheet() export.Sheet {
	return export.Sheet{
		Name:     "待办",
		FileBase: "tasks",
		Headers:  []string{"id", "标题", "备注"},
		Rows: [][]string{
			{"0000000000000000000001", `带,逗号`, "普通"},
			{"0000000000000000000002", `带|竖线`, "带\"引号\""},
			{"0000000000000000000003", "多行", "第一行\n第二行"},
		},
	}
}

func TestRoundTripThroughEveryFormat(t *testing.T) {
	// The point of the pair: what the exporter wrote, the importer reads back
	// unchanged. A format that cannot survive this is a format whose export is a
	// trap.
	for _, layout := range []export.Layout{export.LayoutCSV, export.LayoutMarkdown, export.LayoutXLSX} {
		sheet := trickySheet()
		data, err := infra.Encode(layout, []export.Sheet{sheet})
		if err != nil {
			t.Fatalf("%v: encode: %v", layout, err)
		}
		documents, err := infra.Decode(layout, data)
		if err != nil {
			t.Fatalf("%v: decode: %v", layout, err)
		}
		if len(documents) != 1 {
			t.Fatalf("%v: got %d documents, want 1", layout, len(documents))
		}
		document := documents[0]
		if len(document.Headers) != len(sheet.Headers) {
			t.Fatalf("%v: headers = %v", layout, document.Headers)
		}
		for i, header := range sheet.Headers {
			if document.Headers[i] != header {
				t.Fatalf("%v: header %d = %q, want %q", layout, i, document.Headers[i], header)
			}
		}
		if len(document.Rows) != len(sheet.Rows) {
			t.Fatalf("%v: got %d rows, want %d", layout, len(document.Rows), len(sheet.Rows))
		}
		for row := range sheet.Rows {
			for column := range sheet.Headers {
				if got, want := document.Cell(row, column), sheet.Rows[row][column]; got != want {
					t.Fatalf("%v: cell %d,%d = %q, want %q", layout, row, column, got, want)
				}
			}
		}
	}
}

func TestDecodeReadsAHandEditedCSV(t *testing.T) {
	// A file that went through a spreadsheet arrives with a byte-order mark, a
	// short last row and unquoted values.
	data := []byte("\ufeffid,title\n1,写周报\n2\n")
	documents, err := infra.Decode(export.LayoutCSV, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	document := documents[0]
	if document.Headers[0] != "id" || document.Headers[1] != "title" {
		t.Fatalf("headers = %v", document.Headers)
	}
	if document.Cell(0, 1) != "写周报" {
		t.Fatalf("cell = %q", document.Cell(0, 1))
	}
	if document.Cell(1, 1) != "" {
		t.Fatalf("a short row must read as empty, got %q", document.Cell(1, 1))
	}
}

func TestDecodeReadsTheFirstMarkdownTableOnly(t *testing.T) {
	data := []byte("# 导出\n\n## 待办\n\n| id | 标题 |\n| --- | --- |\n| 1 | 写周报 |\n\n后面的文字\n")
	documents, err := infra.Decode(export.LayoutMarkdown, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(documents) != 1 || len(documents[0].Rows) != 1 {
		t.Fatalf("documents = %+v", documents)
	}
	if documents[0].Cell(0, 1) != "写周报" {
		t.Fatalf("cell = %q", documents[0].Cell(0, 1))
	}
}

func TestDecodeReadsEverySheetOfAWorkbook(t *testing.T) {
	sheets := []export.Sheet{
		{Name: "待办", FileBase: "tasks", Headers: []string{"id"}, Rows: [][]string{{"1"}}},
		{Name: "账单", FileBase: "entries", Headers: []string{"id", "金额"}, Rows: [][]string{{"2", "-3.50"}}},
	}
	data, err := infra.Encode(export.LayoutXLSX, sheets)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	documents, err := infra.Decode(export.LayoutXLSX, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(documents) != 2 {
		t.Fatalf("got %d documents, want 2", len(documents))
	}
	if documents[1].Name != "账单" {
		t.Fatalf("second sheet name = %q", documents[1].Name)
	}
	if documents[1].Cell(0, 1) != "-3.50" {
		t.Fatalf("amount = %q", documents[1].Cell(0, 1))
	}
}

func TestDecodeRefusesWhatItCannotRead(t *testing.T) {
	if _, err := infra.Decode(export.LayoutCSV, nil); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an empty file must be refused, got %v", err)
	}
	if _, err := infra.Decode(export.LayoutXLSX, []byte("not a workbook")); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a non-workbook must be refused, got %v", err)
	}
	if _, err := infra.Decode(export.LayoutMarkdown, []byte("no table here")); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a file with no table must be refused, got %v", err)
	}
	if _, err := infra.Decode(export.Layout(99), []byte("x")); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unknown layout must be refused, got %v", err)
	}
}
