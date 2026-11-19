package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// ── times ─────────────────────────────────────────────────────────────
//
// Every instant is stored as Unix milliseconds, with 0 meaning unset.

// ms converts an instant to the stored integer.
func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// toTime converts the stored integer back, in local time: the app is local-first
// and every date the user sees is a local date.
func toTime(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).Local()
}

// durMs stores a duration as milliseconds; negative durations are meaningless
// here and stored as 0.
func durMs(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return int64(d / time.Millisecond)
}

func toDuration(v int64) time.Duration { return time.Duration(v) * time.Millisecond }

// ── calendar days ──────────────────────────────────────────────────────

// date stores a LocalDate as 'YYYY-MM-DD', or "" when unset. A text column keeps
// the value readable in a sqlite3 shell and sorts correctly.
func date(d shared.LocalDate) string {
	if d.IsZero() {
		return ""
	}
	return d.String()
}

func toDate(s string) (shared.LocalDate, error) {
	if s == "" {
		return shared.LocalDate{}, nil
	}
	return shared.ParseLocalDate(s)
}

// month stores a Month as 'YYYY-MM'.
func month(m shared.Month) string {
	if m.IsZero() {
		return ""
	}
	return m.String()
}

func toMonth(s string) (shared.Month, error) {
	if s == "" {
		return shared.Month{}, nil
	}
	return shared.ParseMonth(s)
}

// ── tags ───────────────────────────────────────────────────────────────

// tags encodes a tag list as a JSON array. JSON rather than a separator keeps a tag
// that contains a comma from splitting in two.
func tags(list []string) string {
	if len(list) == 0 {
		return "[]"
	}
	b, err := json.Marshal(list)
	if err != nil {
		// A []string always marshals; the fallback only exists so the signature
		// stays error-free at the call site.
		return "[]"
	}
	return string(b)
}

func toTags(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		// Corrupt JSON must not make a row unreadable: the tags are decoration,
		// the row is the data.
		return nil
	}
	return out
}

// weekdays encodes a weekday list as a JSON array of ints.
func weekdays(list []time.Weekday) string {
	if len(list) == 0 {
		return "[]"
	}
	out := make([]int, len(list))
	for i, d := range list {
		out[i] = int(d)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func toWeekdays(s string) []time.Weekday {
	if s == "" || s == "[]" {
		return nil
	}
	var ints []int
	if err := json.Unmarshal([]byte(s), &ints); err != nil {
		return nil
	}
	out := make([]time.Weekday, 0, len(ints))
	for _, v := range ints {
		out = append(out, time.Weekday(v))
	}
	return out
}

// dates encodes a date list as a JSON array of 'YYYY-MM-DD' strings.
func dates(list []shared.LocalDate) string {
	if len(list) == 0 {
		return "[]"
	}
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = d.String()
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func toDates(s string) []shared.LocalDate {
	if s == "" || s == "[]" {
		return nil
	}
	var strs []string
	if err := json.Unmarshal([]byte(s), &strs); err != nil {
		return nil
	}
	out := make([]shared.LocalDate, 0, len(strs))
	for _, v := range strs {
		if d, err := shared.ParseLocalDate(v); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// ── money ──────────────────────────────────────────────────────────────

// toCurrency reads a stored currency code, falling back to the default rather than
// failing the whole query over one bad byte.
func toCurrency(s string) shared.Currency {
	c := shared.Currency(s)
	if !c.Valid() {
		return shared.DefaultCurrency
	}
	return c
}

// nullable turns an empty ID into SQL NULL, for the columns that reference another
// row: an empty string would be a foreign key pointing at nothing.
func nullable(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return string(id)
}

// idOrEmpty reads a nullable identifier column.
func idOrEmpty(ns sql.NullString) shared.ID {
	if !ns.Valid {
		return ""
	}
	return shared.ID(ns.String)
}

// boolToInt and intToBool convert the 0/1 columns.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(v int) bool { return v != 0 }
