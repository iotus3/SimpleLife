package shared

import "time"

// Clock is the only source of "now" the domain may use. Injecting it makes the
// pomodoro timing, the daily views and the template expansion reproducible in
// tests: no assertion ever depends on the wall clock of the machine running it.
type Clock interface {
	Now() time.Time
}

// Today returns the clock's current calendar day.
func Today(c Clock) LocalDate { return DateOf(c.Now()) }

// ThisMonth returns the clock's current calendar month.
func ThisMonth(c Clock) Month { return MonthOf(c.Now()) }

// ClockFunc adapts a function to Clock, handy in tests and in adapters.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }
