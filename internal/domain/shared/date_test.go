package shared

import (
	"testing"
	"time"
)

func TestLocalDateParseAndString(t *testing.T) {
	d, err := ParseLocalDate("2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	if d.Year != 2026 || d.Month != time.November || d.Day != 17 {
		t.Fatalf("parsed %+v", d)
	}
	if d.String() != "2026-11-17" {
		t.Fatalf("String() = %q", d.String())
	}
	if _, err := ParseLocalDate("2026-13-01"); err == nil {
		t.Error("month 13 must fail")
	}
	if _, err := ParseLocalDate("17/11/2026"); err == nil {
		t.Error("a non-ISO layout must fail")
	}
	if (LocalDate{}).IsZero() != true {
		t.Error("zero value must report IsZero")
	}
	if (LocalDate{}).String() != "" {
		t.Error("zero value must render as empty string")
	}
}

func TestLocalDateAddDaysCrossesBoundaries(t *testing.T) {
	cases := []struct {
		from string
		n    int
		want string
	}{
		{"2026-11-17", 1, "2026-11-18"},
		{"2026-11-30", 1, "2026-12-01"},
		{"2026-12-31", 1, "2027-01-01"},
		{"2027-01-01", -1, "2026-12-31"},
		{"2024-02-28", 1, "2024-02-29"}, // leap year
		{"2023-02-28", 1, "2023-03-01"}, // common year
		{"2024-02-29", 1, "2024-03-01"},
		{"2026-03-01", -1, "2026-02-28"},
		{"2026-11-17", 0, "2026-11-17"},
		{"2026-11-17", 365, "2027-11-17"},
	}
	for _, c := range cases {
		got := MustParseLocalDate(c.from).AddDays(c.n)
		if got.String() != c.want {
			t.Errorf("%s + %d days = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}

func TestLocalDateComparison(t *testing.T) {
	a := MustParseLocalDate("2026-11-17")
	b := MustParseLocalDate("2026-11-18")
	if !a.Before(b) || b.Before(a) {
		t.Error("ordering is wrong")
	}
	if a.Equal(b) || !a.Equal(MustParseLocalDate("2026-11-17")) {
		t.Error("equality is wrong")
	}
	if !MustParseLocalDate("2026-12-01").After(a) {
		t.Error("month rollover ordering is wrong")
	}
}

func TestLocalDateBoundsAreInclusive(t *testing.T) {
	d := MustParseLocalDate("2026-11-17")
	start, end := d.Midnight(), d.EndOfDay()
	if start.Hour() != 0 || start.Minute() != 0 {
		t.Fatalf("midnight = %v", start)
	}
	if DateOf(start) != d || DateOf(end) != d {
		t.Fatalf("both bounds must land on %s: %v %v", d, start, end)
	}
	if end.Sub(start) < 23*time.Hour+59*time.Minute {
		t.Fatalf("end of day is too early: %v", end.Sub(start))
	}
}

func TestLocalDateDaysInMonth(t *testing.T) {
	cases := map[string]int{
		"2024-02-01": 29,
		"2023-02-01": 28,
		"2026-04-01": 30,
		"2026-12-01": 31,
	}
	for in, want := range cases {
		if got := MustParseLocalDate(in).DaysInMonth(); got != want {
			t.Errorf("%s has %d days, want %d", in, got, want)
		}
	}
}

func TestMonthParseAddAndBounds(t *testing.T) {
	m := MustParseMonth("2026-11")
	if m.String() != "2026-11" {
		t.Fatalf("String() = %q", m.String())
	}
	if got := m.AddMonths(2).String(); got != "2027-01" {
		t.Errorf("AddMonths(2) = %s, want 2027-01", got)
	}
	if got := m.AddMonths(-11).String(); got != "2025-12" {
		t.Errorf("AddMonths(-11) = %s, want 2025-12", got)
	}
	if got := MonthOf(m.Start()).String(); got != "2026-11" {
		t.Errorf("Start month = %s", got)
	}
	if got := MonthOf(m.End()).String(); got != "2026-11" {
		t.Errorf("End must stay inside the month, got %s", got)
	}
	if n := len(m.Days()); n != 30 {
		t.Errorf("November has %d days, want 30", n)
	}
	if _, err := ParseMonth("2026-13"); err == nil {
		t.Error("month 13 must fail")
	}
	if _, err := ParseMonth("2026-1"); err == nil {
		t.Error("unpadded month must fail")
	}
}

func TestDateRangeIsHalfOpen(t *testing.T) {
	r := OfMonth(MustParseMonth("2026-11"))
	if !r.Contains(MustParseLocalDate("2026-11-01")) {
		t.Error("the first day belongs to the month")
	}
	if r.Contains(MustParseLocalDate("2026-12-01")) {
		t.Error("the first day of next month must not belong to the range")
	}
	if r.Contains(MustParseLocalDate("2026-10-31")) {
		t.Error("the last day of the previous month must not belong")
	}
	if !(DateRange{}).IsZero() {
		t.Error("zero range must report IsZero")
	}
	if !(DateRange{From: MustParseLocalDate("2026-11-17")}).Contains(MustParseLocalDate("2030-01-01")) {
		t.Error("an open-ended range must contain later dates")
	}
}
