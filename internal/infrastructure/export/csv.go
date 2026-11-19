package export

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"github.com/iotus3/simplelife/app/internal/domain/export"
)

// utf8BOM is the byte order mark Windows Excel needs to read a UTF-8 CSV
// as UTF-8. Without it, double-clicking the file in Excel renders every Chinese
// character as mojibake — the single most common "the export is broken" report.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// encodeCSV writes one sheet as RFC 4180 CSV with a BOM and CRLF line endings.
func encodeCSV(sheet export.Sheet) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(utf8BOM)
	writer := csv.NewWriter(&buf)
	// CRLF is what RFC 4180 specifies and what Excel expects.
	writer.UseCRLF = true
	if err := writer.Write(sheet.Headers); err != nil {
		return nil, fmt.Errorf("export: csv header: %w", err)
	}
	for _, row := range sheet.Rows {
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("export: csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("export: csv flush: %w", err)
	}
	return buf.Bytes(), nil
}
