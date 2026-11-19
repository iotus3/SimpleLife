package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// dateLayout is the canonical date format used by storage, exports and URLs.
const dateLayout = "2006-01-02"

// LocalDate is a calendar day in the user's own time zone. Grouping tasks and
// ledger entries by "today" must not depend on UTC: a session finished at 00:30
// China time belongs to that day even though UTC still says yesterday.
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar day t falls on, in t's own location.
func DateOf(t time.Time) LocalDate {
	y, m, d := t.Date()
	return LocalDate{Year: y, Month: m, Day: d}
}

// NewLocalDate builds a date, normalising overflow the way time.Date does.
func NewLocalDate(year int, month time.Month, day int) LocalDate {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	return DateOf(t)
}

// ParseLocalDate parses "YYYY-MM-DD".
func ParseLocalDate(s string) (LocalDate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return LocalDate{}, Invalid("date.empty", "", "date is empty")
	}
	t, err := time.ParseInLocation(dateLayout, s, time.Local)
	if err != nil {
		return LocalDate{}, Invalid("date.format", "", "date must be YYYY-MM-DD: "+s)
	}
	return DateOf(t), nil
}

// MustParseLocalDate is ParseLocalDate for tests and constants.
func MustParseLocalDate(s string) LocalDate {
	d, err := ParseLocalDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// String renders "YYYY-MM-DD".
func (d LocalDate) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// IsZero reports the unset date (used for "no due date").
func (d LocalDate) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Before reports d < o.
func (d LocalDate) Before(o LocalDate) bool { return d.compare(o) < 0 }

// After reports d > o.
func (d LocalDate) After(o LocalDate) bool { return d.compare(o) > 0 }

// Equal reports the same calendar day.
func (d LocalDate) Equal(o LocalDate) bool { return d.compare(o) == 0 }

func (d LocalDate) compare(o LocalDate) int {
	switch {
	case d.Year != o.Year:
		if d.Year < o.Year {
			return -1
		}
		return 1
	case d.Month != o.Month:
		if d.Month < o.Month {
			return -1
		}
		return 1
	case d.Day != o.Day:
		if d.Day < o.Day {
			return -1
		}
		return 1
	}
	return 0
}

// Midnight returns the start of the day in local time.
func (d LocalDate) Midnight() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.Local)
}

// EndOfDay returns the last instant of the day in local time. Task due dates use
// it, so "due today" stays valid for the whole day.
func (d LocalDate) EndOfDay() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 23, 59, 59, int(999*time.Millisecond), time.Local)
}

// AddDays returns the date n days later (negative goes back), crossing months,
// years and leap days correctly.
func (d LocalDate) AddDays(n int) LocalDate {
	return DateOf(d.Midnight().AddDate(0, 0, n))
}

// Weekday returns the day of week.
func (d LocalDate) Weekday() time.Weekday { return d.Midnight().Weekday() }

// At returns the given time of day on that date.
func (d LocalDate) At(hour, minute int) time.Time {
	return time.Date(d.Year, d.Month, d.Day, hour, minute, 0, 0, time.Local)
}

// DaysInMonth returns how many days the month of d has.
func (d LocalDate) DaysInMonth() int {
	return time.Date(d.Year, d.Month+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// AsMonth returns the calendar month d belongs to.
func (d LocalDate) AsMonth() Month { return Month{d.Year, d.Month} }

// DaysSince returns the whole number of calendar days from o to d, negative when d is
// earlier and 0 when either side is unset. Calendar days, not 24-hour spans, so a
// daylight-saving shift never turns "tomorrow" into "today".
func (d LocalDate) DaysSince(o LocalDate) int {
	if d.IsZero() || o.IsZero() {
		return 0
	}
	// Noon in UTC: no zone has a DST transition at 12:00, so the difference is a
	// whole number of days on every platform.
	a := time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC)
	b := time.Date(o.Year, o.Month, o.Day, 12, 0, 0, 0, time.UTC)
	return int(a.Sub(b) / (24 * time.Hour))
}

// Month is a calendar month, the period budgets use.
type Month struct {
	Year  int
	Month time.Month
}

// MonthOf returns the month t falls in.
func MonthOf(t time.Time) Month {
	y, m, _ := t.Date()
	return Month{Year: y, Month: m}
}

// ParseMonth parses "YYYY-MM".
func ParseMonth(s string) (Month, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 || len(parts[0]) != 4 || len(parts[1]) != 2 {
		return Month{}, Invalid("month.format", "", "month must be YYYY-MM: "+s)
	}
	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return Month{}, Invalid("month.format", "", "month must be YYYY-MM: "+s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 1 || m > 12 {
		return Month{}, Invalid("month.format", "", "month must be YYYY-MM: "+s)
	}
	return Month{Year: y, Month: time.Month(m)}, nil
}

// MustParseMonth is ParseMonth for tests and constants.
func MustParseMonth(s string) Month {
	m, err := ParseMonth(s)
	if err != nil {
		panic(err)
	}
	return m
}

// String renders "YYYY-MM".
func (m Month) String() string { return fmt.Sprintf("%04d-%02d", m.Year, int(m.Month)) }

// IsZero reports the unset month.
func (m Month) IsZero() bool { return m.Year == 0 }

// Start returns the first instant of the month.
func (m Month) Start() time.Time { return time.Date(m.Year, m.Month, 1, 0, 0, 0, 0, time.Local) }

// End returns the last instant of the month.
func (m Month) End() time.Time {
	return time.Date(m.Year, m.Month+1, 1, 0, 0, 0, 0, time.Local).Add(-time.Millisecond)
}

// AddMonths returns the month n later (negative goes back).
func (m Month) AddMonths(n int) Month {
	t := m.Start().AddDate(0, n, 0)
	return MonthOf(t)
}

// Days returns every date of the month.
func (m Month) Days() []LocalDate {
	out := make([]LocalDate, 0, 31)
	for d := m.Start(); d.Month() == m.Month && d.Year() == m.Year; d = d.AddDate(0, 0, 1) {
		out = append(out, DateOf(d))
	}
	return out
}

// DateRange is a half-open interval [From, To) of calendar days. The zero value
// means unbounded on either side.
type DateRange struct {
	From LocalDate
	To   LocalDate
}

// Contains reports whether d falls in the range.
func (r DateRange) Contains(d LocalDate) bool {
	if !r.From.IsZero() && d.Before(r.From) {
		return false
	}
	if !r.To.IsZero() && !d.Before(r.To) {
		return false
	}
	return true
}

// IsZero reports an unbounded range.
func (r DateRange) IsZero() bool { return r.From.IsZero() && r.To.IsZero() }

// Start returns the first instant of the range.
func (r DateRange) Start() time.Time {
	if r.From.IsZero() {
		return time.Time{}
	}
	return r.From.Midnight()
}

// End returns the exclusive end instant of the range.
func (r DateRange) End() time.Time {
	if r.To.IsZero() {
		return time.Time{}
	}
	return r.To.Midnight()
}

// OfMonth returns the range covering m.
func OfMonth(m Month) DateRange {
	return DateRange{From: DateOf(m.Start()), To: DateOf(m.AddMonths(1).Start())}
}
