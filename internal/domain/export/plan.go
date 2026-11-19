// Package export describes what to export and what the encoded rows look like, and nothing
// about how they are encoded.
package export

import (
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Layout is the output format.
type Layout uint8

const (
	LayoutCSV Layout = iota + 1
	LayoutMarkdown
	LayoutXLSX
)

// String returns the stable storage/CLI key.
func (l Layout) String() string {
	switch l {
	case LayoutCSV:
		return "csv"
	case LayoutMarkdown:
		return "markdown"
	case LayoutXLSX:
		return "xlsx"
	}
	return "unknown"
}

// ParseLayout is the inverse of String.
func ParseLayout(s string) (Layout, error) {
	switch s {
	case "csv":
		return LayoutCSV, nil
	case "markdown", "md":
		return LayoutMarkdown, nil
	case "xlsx":
		return LayoutXLSX, nil
	}
	return 0, shared.Invalid("export.layout.parse", "layout", "unknown layout: "+s)
}

// Extension is the file suffix the save dialog suggests.
func (l Layout) Extension() string {
	switch l {
	case LayoutCSV:
		return "csv"
	case LayoutMarkdown:
		return "md"
	case LayoutXLSX:
		return "xlsx"
	}
	return "txt"
}

// Plan is the complete description of one export request: pure data, no IO.
type Plan struct {
	Tables          []TableSelection
	Range           shared.DateRange
	Currency        shared.Currency // empty = no currency filter
	IncludeArchived bool
	Layout          Layout
}

// TableSelection picks one table and the columns to emit, in order.
type TableSelection struct {
	Key     string
	Columns []string
}

// Sheet is the materialised result: a header plus rows of strings, shared by all three
// formats so they cannot disagree about a date or an amount.
type Sheet struct {
	// Name is the header text: the sheet name in XLSX, the markdown title.
	Name string
	// FileBase is the ASCII-only base name used for filenames and zip entries.
	// Chinese sheet names in a file name turn into mojibake after a round trip
	// through mail or a zip on another locale.
	FileBase string
	Headers  []string
	Rows     [][]string
}

// Column is one selectable field of a table.
type Column struct {
	Key string
	// Money marks a column whose value is a signed minor-unit amount; the
	// formatter adds two decimals and the currency sign.
	Money bool
	// MoneyCurrency names the currency column that goes with it, if any.
	MoneyCurrency string
	// Time marks an ISO date-time column (never localised, so exports stay
	// sortable and machine readable).
	Time bool
	// Always is set for the columns that identify a row; the UI cannot untick
	// them, so an export always says what each row is.
	Always bool
}

// Table describes one exportable table.
type Table struct {
	Key      string
	FileBase string
	Columns  []Column
}

// DefaultTables is the catalogue of exportable data. Column keys double as the
// message ids for the header row, so a new column needs a translation entry in
// both languages.
func DefaultTables() []Table {
	return []Table{
		{
			Key:      "tasks",
			FileBase: "tasks",
			Columns: []Column{
				{Key: "id", Always: true},
				{Key: "title", Always: true},
				{Key: "status"},
				{Key: "priority"},
				{Key: "due_date"},
				{Key: "estimate_pomodoros"},
				{Key: "focus_minutes"},
				{Key: "done_pomodoros"},
				{Key: "tags"},
				{Key: "notes"},
				{Key: "created_at", Time: true},
				{Key: "completed_at", Time: true},
				{Key: "template_id"},
			},
		},
		{
			Key:      "sessions",
			FileBase: "sessions",
			Columns: []Column{
				{Key: "id", Always: true},
				{Key: "kind", Always: true},
				{Key: "phase"},
				{Key: "title", Always: true},
				{Key: "started_at", Time: true},
				{Key: "ended_at", Time: true},
				{Key: "planned_minutes"},
				{Key: "elapsed_minutes"},
				{Key: "interruptions"},
				{Key: "note"},
			},
		},
		{
			Key:      "entries",
			FileBase: "entries",
			Columns: []Column{
				{Key: "id", Always: true},
				{Key: "kind", Always: true},
				{Key: "amount", Money: true, MoneyCurrency: "currency"},
				{Key: "currency", Always: true},
				{Key: "account", Always: true},
				{Key: "to_account"},
				{Key: "category"},
				{Key: "occurred_at", Time: true},
				{Key: "origin"},
				{Key: "note"},
				{Key: "tags"},
			},
		},
		{
			Key:      "accounts",
			FileBase: "accounts",
			Columns: []Column{
				{Key: "name", Always: true},
				{Key: "kind"},
				{Key: "currency"},
				{Key: "balance", Money: true, MoneyCurrency: "currency"},
				{Key: "archived"},
			},
		},
		{
			Key:      "categories",
			FileBase: "categories",
			Columns: []Column{
				{Key: "name", Always: true},
				{Key: "direction"},
				{Key: "parent"},
			},
		},
		{
			Key:      "budgets",
			FileBase: "budgets",
			Columns: []Column{
				{Key: "month", Always: true},
				{Key: "category", Always: true},
				{Key: "limit", Money: true, MoneyCurrency: "currency"},
				{Key: "currency"},
			},
		},
		{
			Key:      "templates",
			FileBase: "templates",
			Columns: []Column{
				{Key: "name", Always: true},
				{Key: "enabled"},
				{Key: "rule"},
				{Key: "task", Always: true},
				{Key: "notes"},
				{Key: "estimate"},
				{Key: "focus_minutes"},
				{Key: "due_offset"},
			},
		},
	}
}

// TableByKey finds a table definition.
func TableByKey(key string) (Table, bool) {
	for _, t := range DefaultTables() {
		if t.Key == key {
			return t, true
		}
	}
	return Table{}, false
}

// Validate checks that every selected table and column exists and that at least one
// table with at least one column was requested.
func (p Plan) Validate() error {
	if len(p.Tables) == 0 {
		return shared.Invalid("export.tables.empty", "tables", "select at least one table")
	}
	switch p.Layout {
	case LayoutCSV, LayoutMarkdown, LayoutXLSX:
	default:
		return shared.Invalid("export.layout.range", "layout", "unknown layout")
	}
	if p.Currency != "" && !p.Currency.Valid() {
		return shared.Invalid("export.currency", "currency", "currency must be CNY or USD")
	}
	seen := make(map[string]bool, len(p.Tables))
	for _, sel := range p.Tables {
		if seen[sel.Key] {
			return shared.Invalid("export.tables.duplicate", "tables", "table selected twice: "+sel.Key)
		}
		seen[sel.Key] = true
		table, ok := TableByKey(sel.Key)
		if !ok {
			return shared.Invalid("export.table.unknown", "tables", "unknown table: "+sel.Key)
		}
		if len(sel.Columns) == 0 {
			return shared.Invalid("export.columns.empty", "columns", "select at least one column for "+sel.Key)
		}
		known := make(map[string]bool, len(table.Columns))
		for _, c := range table.Columns {
			known[c.Key] = true
		}
		for _, key := range sel.Columns {
			if !known[key] {
				return shared.Invalid("export.column.unknown", "columns", "unknown column: "+key)
			}
		}
	}
	// CSV and Markdown carry one table per file; several tables would have to be
	// concatenated, which no consumer expects. The UI enforces the same rule.
	if len(p.Tables) > 1 && (p.Layout == LayoutCSV || p.Layout == LayoutMarkdown) {
		return shared.Invalid("export.tables.multiple", "tables", "CSV and Markdown hold one table; pick XLSX for several")
	}
	return nil
}

// FileName is the suggested download name: ASCII only, date-stamped so repeated
// exports do not silently overwrite each other.
func (p Plan) FileName(day shared.LocalDate) string {
	base := "simplelife"
	if len(p.Tables) == 1 {
		if t, ok := TableByKey(p.Tables[0].Key); ok && t.FileBase != "" {
			base = "simplelife-" + t.FileBase
		}
	} else {
		base = "simplelife-export"
	}
	if !day.IsZero() {
		base += "-" + strings.ReplaceAll(day.String(), "-", "")
	}
	return base + "." + p.Layout.Extension()
}
