package format

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

func TestMoneyFollowsTheCurrencyNotTheLanguage(t *testing.T) {
	cny := shared.MustParseMoney("1234.50", shared.CNY)
	usd := shared.MustParseMoney("1234.50", shared.USD)
	if got, want := Money(cny), "¥1234.50"; got != want {
		t.Errorf("Money(CNY) = %q, want %q", got, want)
	}
	if got, want := Money(usd), "$1234.50"; got != want {
		t.Errorf("Money(USD) = %q, want %q", got, want)
	}
	negative := shared.MustParseMoney("-0.75", shared.USD)
	if got, want := Money(negative), "-$0.75"; got != want {
		t.Errorf("Money(negative) = %q, want %q", got, want)
	}
	if got, want := MoneySigned(negative), "-$0.75"; got != want {
		t.Errorf("MoneySigned(negative) = %q, want %q", got, want)
	}
	if got, want := MoneySigned(cny), "+¥1234.50"; got != want {
		t.Errorf("MoneySigned(positive) = %q, want %q", got, want)
	}
	if got, want := MoneySigned(shared.ZeroMoney(shared.CNY)), "¥0.00"; got != want {
		t.Errorf("MoneySigned(zero) = %q, want %q", got, want)
	}
}

func TestTotalsNeverMixCurrencies(t *testing.T) {
	totals := []shared.MoneyTotal{
		{Currency: shared.CNY, Amount: shared.MustParseMoney("120.00", shared.CNY)},
		{Currency: shared.USD, Amount: shared.MustParseMoney("8.50", shared.USD)},
	}
	if got, want := Totals(i18n.En, totals), "¥120.00 · $8.50"; got != want {
		t.Errorf("Totals = %q, want %q", got, want)
	}
	if got, want := Totals(i18n.En, nil), "—"; got != want {
		t.Errorf("Totals(nil) = %q, want %q", got, want)
	}
}

func TestClock(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00"},
		{25 * time.Minute, "25:00"},
		{90 * time.Second, "01:30"},
		{time.Hour + 5*time.Minute + 3*time.Second, "01:05:03"},
		{-time.Minute, "00:00"},
	}
	for _, c := range cases {
		if got := Clock(c.in); got != c.want {
			t.Errorf("Clock(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDuration(t *testing.T) {
	if got, want := Duration(i18n.En, 25*time.Minute), "25 min"; got != want {
		t.Errorf("Duration(en) = %q, want %q", got, want)
	}
	if got, want := Duration(i18n.ZhCN, 25*time.Minute), "25 分钟"; got != want {
		t.Errorf("Duration(zh) = %q, want %q", got, want)
	}
	if got, want := Duration(i18n.En, 90*time.Minute), "1 h 30 min"; got != want {
		t.Errorf("Duration(90m) = %q, want %q", got, want)
	}
	if got, want := Duration(i18n.En, 2*time.Hour), "2 h"; got != want {
		t.Errorf("Duration(2h) = %q, want %q", got, want)
	}
	if got, want := Duration(i18n.En, 5*time.Second), "0 min"; got != want {
		t.Errorf("Duration(5s) = %q, want %q", got, want)
	}
}

func TestDateFollowsTheLanguage(t *testing.T) {
	day := shared.NewLocalDate(2026, time.November, 17)
	if got, want := Date(i18n.ZhCN, day), "2026-11-17"; got != want {
		t.Errorf("Date(zh) = %q, want %q", got, want)
	}
	if got, want := Date(i18n.En, day), "Nov 17, 2026"; got != want {
		t.Errorf("Date(en) = %q, want %q", got, want)
	}
	if got, want := Date(i18n.En, shared.LocalDate{}), "—"; got != want {
		t.Errorf("Date(zero) = %q, want %q", got, want)
	}
}

func TestRelativeDay(t *testing.T) {
	today := shared.NewLocalDate(2026, time.November, 17)
	cases := []struct {
		day  shared.LocalDate
		lang i18n.Lang
		want string
	}{
		{today, i18n.En, "Today"},
		{today.AddDays(-1), i18n.En, "Yesterday"},
		{today.AddDays(1), i18n.En, "Tomorrow"},
		{today.AddDays(3), i18n.En, "Nov 20, 2026"},
		{today, i18n.ZhCN, "今天"},
	}
	for _, c := range cases {
		if got := RelativeDay(c.lang, c.day, today); got != c.want {
			t.Errorf("RelativeDay(%v, %v) = %q, want %q", c.lang, c.day, got, c.want)
		}
	}
}

func TestMonthLabel(t *testing.T) {
	month := shared.Month{Year: 2026, Month: time.November}
	if got, want := MonthLabel(i18n.ZhCN, month), "2026-11"; got != want {
		t.Errorf("MonthLabel(zh) = %q, want %q", got, want)
	}
	if got, want := MonthLabel(i18n.En, month), "November 2026"; got != want {
		t.Errorf("MonthLabel(en) = %q, want %q", got, want)
	}
}

func TestWeekdays(t *testing.T) {
	days := []time.Weekday{time.Monday, time.Wednesday}
	if got, want := Weekdays(i18n.En, days), "Mon, Wed"; got != want {
		t.Errorf("Weekdays(en) = %q, want %q", got, want)
	}
	if got, want := Weekdays(i18n.ZhCN, days), "周一, 周三"; got != want {
		t.Errorf("Weekdays(zh) = %q, want %q", got, want)
	}
	if got, want := Weekdays(i18n.En, nil), "—"; got != want {
		t.Errorf("Weekdays(nil) = %q, want %q", got, want)
	}
}

func TestPercentAndPriority(t *testing.T) {
	if got, want := Percent(0.625), "63%"; got != want {
		t.Errorf("Percent = %q, want %q", got, want)
	}
	if got, want := Percent(-1), "0%"; got != want {
		t.Errorf("Percent(negative) = %q, want %q", got, want)
	}
	if got, want := Priority(i18n.En, 4), "Urgent"; got != want {
		t.Errorf("Priority(4) = %q, want %q", got, want)
	}
	if got, want := Priority(i18n.En, 9), "Unset"; got != want {
		t.Errorf("Priority(out of range) = %q, want %q", got, want)
	}
}

func TestTagsAndSplit(t *testing.T) {
	if got, want := Tags([]string{"a", "b"}), "#a #b"; got != want {
		t.Errorf("Tags = %q, want %q", got, want)
	}
	if got := Tags(nil); got != "" {
		t.Errorf("Tags(nil) = %q, want empty", got)
	}
	got := SplitTags(" a, b ，c ,, ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("SplitTags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitTags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseDate(t *testing.T) {
	day, err := ParseDate(i18n.ZhCN, "2026-11-17")
	if err != nil || !day.Equal(shared.NewLocalDate(2026, time.November, 17)) {
		t.Errorf("ParseDate(zh) = %v, %v", day, err)
	}
	day, err = ParseDate(i18n.En, "Nov 17, 2026")
	if err != nil || !day.Equal(shared.NewLocalDate(2026, time.November, 17)) {
		t.Errorf("ParseDate(en) = %v, %v", day, err)
	}
	// Both languages accept both shapes: a user pastes what they have.
	day, err = ParseDate(i18n.En, "2026-11-17")
	if err != nil || day.Year != 2026 {
		t.Errorf("ParseDate(mixed) = %v, %v", day, err)
	}
	if day, err := ParseDate(i18n.En, "  "); err != nil || !day.IsZero() {
		t.Errorf("ParseDate(empty) = %v, %v", day, err)
	}
	if _, err := ParseDate(i18n.En, "not a date"); err == nil {
		t.Error("ParseDate(garbage) accepted the value")
	} else if code := shared.Code(err); code != "date.format" {
		t.Errorf("ParseDate(garbage) code = %q, want date.format", code)
	}
}

func TestParseDateList(t *testing.T) {
	days, err := ParseDateList("2026-11-17, 2026-12-01，2026-11-17")
	if err != nil {
		t.Fatalf("ParseDateList: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("ParseDateList = %v, want two unique days", days)
	}
	if !days[0].Equal(shared.NewLocalDate(2026, time.November, 17)) {
		t.Errorf("first day = %v", days[0])
	}
	if _, err := ParseDateList("tomorrow"); err == nil {
		t.Error("ParseDateList accepted a non-date")
	}
}
