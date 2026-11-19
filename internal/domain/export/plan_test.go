package export_test

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestLayoutRoundTrip(t *testing.T) {
	for _, l := range []export.Layout{export.LayoutCSV, export.LayoutMarkdown, export.LayoutXLSX} {
		back, err := export.ParseLayout(l.String())
		if err != nil || back != l {
			t.Fatalf("round trip failed for %v: %v %v", l, back, err)
		}
		if l.Extension() == "" {
			t.Fatalf("%v has no extension", l)
		}
	}
	if back, err := export.ParseLayout("md"); err != nil || back != export.LayoutMarkdown {
		t.Fatalf("md alias failed: %v %v", back, err)
	}
	if _, err := export.ParseLayout("pdf"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown layout must be invalid, got %v", err)
	}
	if got := export.Layout(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	if got := export.Layout(99).Extension(); got != "txt" {
		t.Fatalf("Extension = %q", got)
	}
}

func TestPlanValidate(t *testing.T) {
	ok := export.Plan{
		Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id", "title"}}},
		Layout: export.LayoutCSV,
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}

	bad := []export.Plan{
		{Layout: export.LayoutCSV},
		{Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id"}}}},
		{Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id"}}}, Layout: export.Layout(9)},
		{Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id"}}}, Layout: export.LayoutCSV, Currency: "EUR"},
		{Tables: []export.TableSelection{{Key: "nope", Columns: []string{"id"}}}, Layout: export.LayoutCSV},
		{Tables: []export.TableSelection{{Key: "tasks"}}, Layout: export.LayoutCSV},
		{Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"nope"}}}, Layout: export.LayoutCSV},
		{
			Tables: []export.TableSelection{
				{Key: "tasks", Columns: []string{"id"}},
				{Key: "tasks", Columns: []string{"id"}},
			},
			Layout: export.LayoutXLSX,
		},
		{
			Tables: []export.TableSelection{
				{Key: "tasks", Columns: []string{"id"}},
				{Key: "entries", Columns: []string{"id"}},
			},
			Layout: export.LayoutCSV,
		},
		{
			Tables: []export.TableSelection{
				{Key: "tasks", Columns: []string{"id"}},
				{Key: "entries", Columns: []string{"id"}},
			},
			Layout: export.LayoutMarkdown,
		},
	}
	for i, p := range bad {
		if !shared.IsKind(p.Validate(), shared.KindInvalid) {
			t.Fatalf("bad plan %d accepted", i)
		}
	}

	multi := export.Plan{
		Tables: []export.TableSelection{
			{Key: "tasks", Columns: []string{"id"}},
			{Key: "entries", Columns: []string{"id"}},
		},
		Layout: export.LayoutXLSX,
	}
	if err := multi.Validate(); err != nil {
		t.Fatalf("several tables in one workbook must be allowed: %v", err)
	}
	if err := (export.Plan{Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id"}}}, Layout: export.LayoutCSV, Currency: shared.USD}).Validate(); err != nil {
		t.Fatalf("a currency filter must be allowed: %v", err)
	}
}

func TestPlanFileName(t *testing.T) {
	day := shared.MustParseLocalDate("2026-11-17")
	single := export.Plan{Tables: []export.TableSelection{{Key: "entries", Columns: []string{"id"}}}, Layout: export.LayoutXLSX}
	if got, want := single.FileName(day), "simplelife-entries-20261117.xlsx"; got != want {
		t.Fatalf("FileName = %q, want %q", got, want)
	}
	multi := export.Plan{
		Tables: []export.TableSelection{
			{Key: "tasks", Columns: []string{"id"}},
			{Key: "entries", Columns: []string{"id"}},
		},
		Layout: export.LayoutXLSX,
	}
	if got, want := multi.FileName(day), "simplelife-export-20261117.xlsx"; got != want {
		t.Fatalf("FileName = %q, want %q", got, want)
	}
	if got, want := multi.FileName(shared.LocalDate{}), "simplelife-export.xlsx"; got != want {
		t.Fatalf("FileName without a date = %q, want %q", got, want)
	}
	unknown := export.Plan{Tables: []export.TableSelection{{Key: "nope"}}, Layout: export.LayoutCSV}
	if got, want := unknown.FileName(day), "simplelife-20261117.csv"; got != want {
		t.Fatalf("FileName = %q, want %q", got, want)
	}
}

func TestDefaultTablesAreConsistent(t *testing.T) {
	seen := make(map[string]bool)
	for _, tbl := range export.DefaultTables() {
		if tbl.Key == "" || tbl.FileBase == "" {
			t.Fatalf("table %+v is missing keys", tbl)
		}
		if seen[tbl.Key] {
			t.Fatalf("duplicate table %q", tbl.Key)
		}
		seen[tbl.Key] = true
		if len(tbl.Columns) == 0 {
			t.Fatalf("table %q has no columns", tbl.Key)
		}
		cols := make(map[string]bool, len(tbl.Columns))
		always := 0
		for _, c := range tbl.Columns {
			if c.Key == "" {
				t.Fatalf("table %q has a column without a key", tbl.Key)
			}
			if cols[c.Key] {
				t.Fatalf("table %q has duplicate column %q", tbl.Key, c.Key)
			}
			cols[c.Key] = true
			if c.Money && c.MoneyCurrency == "" {
				t.Fatalf("table %q column %q is money but names no currency column", tbl.Key, c.Key)
			}
			if c.MoneyCurrency != "" && !cols[c.MoneyCurrency] && tbl.Key != "" {
				// The currency column must exist in the same table, in any order.
				found := false
				for _, other := range tbl.Columns {
					if other.Key == c.MoneyCurrency {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("table %q column %q references missing currency column %q", tbl.Key, c.Key, c.MoneyCurrency)
				}
			}
			if c.Always {
				always++
			}
		}
		if always == 0 {
			t.Fatalf("table %q has no always-on column", tbl.Key)
		}
		if _, ok := export.TableByKey(tbl.Key); !ok {
			t.Fatalf("TableByKey cannot find %q", tbl.Key)
		}
	}
	if _, ok := export.TableByKey("nope"); ok {
		t.Fatal("TableByKey must reject an unknown key")
	}
}
