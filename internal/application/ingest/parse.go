package ingest

import (
	"strconv"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// These parsers read exactly what the export writes: an ISO date, "2006-01-02 15:04", a
// signed decimal amount, and a comma separated tag list.

// parseID reads a record id. An empty id is refused: the id is how a second import
// of the same file knows a row is already there.
func parseID(text, what string) (shared.ID, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", shared.Invalid("import.id", "id", "a "+what+" needs its id to be repeatable")
	}
	id := shared.ID(text)
	if id.IsZero() {
		return "", shared.Invalid("import.id", "id", "unreadable "+what+" id: "+text)
	}
	return id, nil
}

// parseInt reads a bounded integer, treating an empty cell as the fallback.
func parseInt(row rowValues, key string, low, high int) (int, error) {
	text := row.get(key)
	if text == "" {
		return low, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, shared.Invalid("import.number", key, key+" is not a number: "+text)
	}
	if value < low || value > high {
		return 0, shared.Invalid("import.number.range", key, key+" is out of range: "+text)
	}
	return value, nil
}

// parseDate reads an ISO date; empty means "no date".
func parseDate(text string) (shared.LocalDate, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return shared.LocalDate{}, nil
	}
	day, err := shared.ParseLocalDate(text)
	if err != nil {
		return shared.LocalDate{}, shared.Invalid("import.date", "date", "unreadable date: "+text).Wrap(err)
	}
	return day, nil
}

// parseTime reads the timestamp the export writes, falling back to a bare date and
// to RFC 3339 so a file edited elsewhere still imports.
func parseTime(text string) (time.Time, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", time.RFC3339, "2006-01-02"} {
		if at, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return at, nil
		}
	}
	return time.Time{}, shared.Invalid("import.time", "time", "unreadable timestamp: "+text)
}

// parseCurrency reads the currency of a row. The export writes it as a column of its
// own, never as a symbol, so a missing value is refused rather than guessed.
func parseCurrency(text string) (shared.Currency, error) {
	currency := shared.Currency(strings.ToUpper(strings.TrimSpace(text)))
	if currency == "" {
		return "", shared.Invalid("import.currency", "currency", "the row has no currency")
	}
	if !currency.Valid() {
		return "", shared.Invalid("import.currency", "currency", "unsupported currency: "+text)
	}
	return currency, nil
}

// parseMoney reads an amount the export wrote — a signed decimal in minor units,
// such as "-12.50" — and returns it positive, because the sign of a ledger line
// lives in its kind, not in its amount.
func parseMoney(text string, currency shared.Currency) (shared.Money, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return shared.Money{}, shared.Invalid("import.amount", "amount", "the row has no amount")
	}
	// The sign says which direction the money moved, and the entry's kind already
	// carries that: only the magnitude is read here.
	switch text[0] {
	case '-', '+':
		text = text[1:]
	}
	whole, fraction := text, ""
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		whole, fraction = text[:dot], text[dot+1:]
	}
	if len(fraction) > 2 {
		fraction = fraction[:2]
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	if whole == "" {
		whole = "0"
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return shared.Money{}, shared.Invalid("import.amount", "amount", "unreadable amount: "+text)
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return shared.Money{}, shared.Invalid("import.amount", "amount", "unreadable amount: "+text)
	}
	minor := units*100 + cents
	if minor <= 0 {
		return shared.Money{}, shared.Invalid("ledger.entry.amount", "amount", "amount must be greater than zero")
	}
	return shared.NewMoney(minor, currency), nil
}

// splitList reads a comma separated cell such as a tag list.
func splitList(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
