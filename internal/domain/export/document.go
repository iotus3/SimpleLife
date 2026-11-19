package export

// Document is one table read back from a file: a header row and the rows beneath it. It
// mirrors Sheet, so both halves of a round trip can be compared value by value.
type Document struct {
	// Name is where the table came from: the sheet name of a workbook, or empty
	// for a file that holds a single table.
	Name string
	// Headers are the cells of the first row, in file order. They may be localized
	// labels or raw column keys; matching them is the importer's job.
	Headers []string
	// Rows are the data rows. A row may be shorter than Headers; the missing cells
	// read as empty.
	Rows [][]string
}

// Cell returns one cell of one row, or "" when the file is ragged.
func (d Document) Cell(row, column int) string {
	if row < 0 || row >= len(d.Rows) {
		return ""
	}
	cells := d.Rows[row]
	if column < 0 || column >= len(cells) {
		return ""
	}
	return cells[column]
}
