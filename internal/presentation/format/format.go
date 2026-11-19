// Package format turns typed values into the strings a user reads: pure functions, each with a
// test. The currency sign follows the currency, never the interface language; dates, month names
// and durations follow the language. Export files never come through here.
package format

import (
	"strconv"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// Money renders an amount with its currency sign: "¥12.34", "-$0.75".
func Money(m shared.Money) string {
	body := m.String()
	if strings.HasPrefix(body, "-") {
		return "-" + m.Currency().Symbol() + strings.TrimPrefix(body, "-")
	}
	return m.Currency().Symbol() + body
}

// MoneySigned renders an amount with an explicit sign, for a column of income
// and expense: "+¥12.34", "-¥12.34", "¥0.00" for zero.
func MoneySigned(m shared.Money) string {
	switch {
	case m.IsNegative():
		return Money(m)
	case m.IsZero():
		return Money(m)
	}
	return "+" + Money(m)
}

// Totals renders one amount per currency, side by side: "¥120.00 · $8.50".
// Currencies are never added together, so the caller always sees which is which.
func Totals(l i18n.Lang, totals []shared.MoneyTotal) string {
	if len(totals) == 0 {
		return i18n.T(l, i18n.MsgNone)
	}
	parts := make([]string, 0, len(totals))
	for _, total := range totals {
		parts = append(parts, Money(total.Amount))
	}
	return strings.Join(parts, " · ")
}

// TotalsIn is Totals with a currency to fall back on: a day with no entry would
// otherwise read as a dash, while the page is still counting in one currency.
func TotalsIn(l i18n.Lang, totals []shared.MoneyTotal, currency shared.Currency) string {
	if len(totals) == 0 {
		return Money(shared.NewMoney(0, currency))
	}
	return Totals(l, totals)
}

// Clock renders a duration as mm:ss, or h:mm:ss past an hour, for the timer.
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return pad2(hours) + ":" + pad2(minutes) + ":" + pad2(seconds)
	}
	return pad2(minutes) + ":" + pad2(seconds)
}

// Duration renders a span the way a person says it: "25 min", "1 h 30 min".
func Duration(l i18n.Lang, d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	if minutes < 1 {
		return "0 " + i18n.T(l, i18n.MsgMinutes)
	}
	hours := minutes / 60
	rest := minutes % 60
	hourUnit := i18n.T(l, i18n.MsgHours)
	if hours == 0 {
		return strconv.Itoa(rest) + " " + i18n.T(l, i18n.MsgMinutes)
	}
	if rest == 0 {
		return strconv.Itoa(hours) + " " + hourUnit
	}
	return strconv.Itoa(hours) + " " + hourUnit + " " + strconv.Itoa(rest) + " " + i18n.T(l, i18n.MsgMinutes)
}

// DateLayout is the date pattern of a language. It is also the pattern the date
// inputs of the forms expect.
func DateLayout(l i18n.Lang) string {
	if l == i18n.En {
		return "Jan 2, 2006"
	}
	return "2006-01-02"
}

// Date renders a calendar day in the language's own style.
func Date(l i18n.Lang, day shared.LocalDate) string {
	if day.IsZero() {
		return i18n.T(l, i18n.MsgNone)
	}
	if l == i18n.En {
		return day.Midnight().Format("Jan 2, 2006")
	}
	return day.String()
}

// RelativeDay renders day against today: "Today" when they match, otherwise the
// date. It is what the header of a day section shows.
func RelativeDay(l i18n.Lang, day, today shared.LocalDate) string {
	switch {
	case day.IsZero():
		return i18n.T(l, i18n.MsgNone)
	case day.Equal(today):
		return i18n.T(l, i18n.MsgToday)
	case day.Equal(today.AddDays(-1)):
		return i18n.T(l, i18n.MsgYesterday)
	case day.Equal(today.AddDays(1)):
		return i18n.T(l, i18n.MsgTomorrow)
	}
	return Date(l, day)
}

// ISOWeek is the week of the year a day falls in, counted the way ISO 8601 and
// most calendars do: weeks start on Monday and week 1 is the one holding the
// first Thursday.
func ISOWeek(day shared.LocalDate) int {
	_, week := day.Midnight().ISOWeek()
	return week
}

// DayBanner spells a day out: its date, its weekday and the week of the year it
// falls in. It is the heading of a page that counts a single day, where "today"
// alone leaves the reader to work out which day the numbers belong to.
func DayBanner(l i18n.Lang, day shared.LocalDate) string {
	if day.IsZero() {
		return i18n.T(l, i18n.MsgNone)
	}
	return i18n.T(l, i18n.MsgHomeDateBanner, Date(l, day), Weekday(l, day.Weekday()), ISOWeek(day))
}

// MonthLabel renders a month: "2026-11" in Chinese, "November 2026" in English.
func MonthLabel(l i18n.Lang, m shared.Month) string {
	if m.IsZero() {
		return i18n.T(l, i18n.MsgNone)
	}
	if l == i18n.En {
		return m.Start().Format("January 2006")
	}
	return m.String()
}

// Weekday renders a short weekday name, for the weekly trigger of a template.
func Weekday(l i18n.Lang, w time.Weekday) string {
	ids := [...]i18n.MessageID{
		time.Sunday:    i18n.MsgWeekdaySun,
		time.Monday:    i18n.MsgWeekdayMon,
		time.Tuesday:   i18n.MsgWeekdayTue,
		time.Wednesday: i18n.MsgWeekdayWed,
		time.Thursday:  i18n.MsgWeekdayThu,
		time.Friday:    i18n.MsgWeekdayFri,
		time.Saturday:  i18n.MsgWeekdaySat,
	}
	if w < time.Sunday || w > time.Saturday {
		return i18n.T(l, i18n.MsgNone)
	}
	return i18n.T(l, ids[w])
}

// Weekdays renders a list of weekdays: "Mon, Wed, Fri".
func Weekdays(l i18n.Lang, days []time.Weekday) string {
	if len(days) == 0 {
		return i18n.T(l, i18n.MsgNone)
	}
	parts := make([]string, 0, len(days))
	for _, day := range days {
		parts = append(parts, Weekday(l, day))
	}
	return strings.Join(parts, ", ")
}

// Percent renders a 0..1 fraction as whole percent: "62%".
func Percent(fraction float64) string {
	if fraction < 0 {
		fraction = 0
	}
	return strconv.Itoa(int(fraction*100+0.5)) + "%"
}

// Priority renders a task priority. Zero means "unset", which the interface
// shows as a muted dash rather than a label.
func Priority(l i18n.Lang, priority int) string {
	switch priority {
	case 0:
		return i18n.T(l, i18n.MsgTaskPriorityUnset)
	case 1:
		return i18n.T(l, i18n.MsgTaskPriorityLow)
	case 2:
		return i18n.T(l, i18n.MsgTaskPriorityMedium)
	case 3:
		return i18n.T(l, i18n.MsgTaskPriorityHigh)
	case 4:
		return i18n.T(l, i18n.MsgTaskPriorityUrgent)
	}
	return i18n.T(l, i18n.MsgTaskPriorityUnset)
}

// Tags renders a tag list as "#a #b".
func Tags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, "#"+tag)
	}
	return strings.Join(parts, " ")
}

// ParseDate reads a date the way the language writes it. An empty value is a
// zero day, which means "no date".
func ParseDate(l i18n.Lang, text string) (shared.LocalDate, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return shared.LocalDate{}, nil
	}
	layouts := []string{DateLayout(l), "2006-01-02", "2006/01/02", "Jan 2, 2006", "01/02/2006"}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, text, time.Local)
		if err == nil {
			return shared.DateOf(parsed), nil
		}
		lastErr = err
	}
	return shared.LocalDate{}, shared.Invalid("date.format", "date", "cannot read the date "+text).Wrap(lastErr)
}

// ParseDateList reads a comma separated list of dates ("2026-11-17, 2026-12-01").
// Days are kept in the order they were typed and duplicates are dropped.
func ParseDateList(text string) ([]shared.LocalDate, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '，' || r == ';' })
	out := make([]shared.LocalDate, 0, len(fields))
	seen := make(map[shared.LocalDate]bool, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) == "" {
			continue
		}
		day, err := ParseDate(i18n.ZhCN, field)
		if err != nil {
			day, err = ParseDate(i18n.En, field)
			if err != nil {
				return nil, err
			}
		}
		if day.IsZero() || seen[day] {
			continue
		}
		seen[day] = true
		out = append(out, day)
	}
	return out, nil
}

// SplitTags turns "a, b ,,c" into ["a" "b" "c"].
func SplitTags(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
