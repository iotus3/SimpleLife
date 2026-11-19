package presentation

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// The statistics page counts a span the user chooses: a week is only the first of
// them, and the other two are one press away.
func TestStatsPageCountsTheChosenSpan(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteStats
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if got := len(fixture.state.statsBars); got != statsRangeWeek {
		t.Fatalf("a fresh statistics page counts %d days, want %d", got, statsRangeWeek)
	}
	if !session.HasText(zh(i18n.MsgStatsRangeWeek)) {
		t.Errorf("the week span is not offered: %v", session.Texts())
	}

	for _, days := range []int{statsRangeFortnight, statsRangeMonth} {
		if err := session.ClickLabel(zh(i18n.MsgStatsRangeDays, days)); err != nil {
			t.Fatalf("switch to %d days: %v", days, err)
		}
		settle(t, session)
		if fixture.state.statsRange != days {
			t.Fatalf("the range is %d after choosing %d", fixture.state.statsRange, days)
		}
		if got := len(fixture.state.statsBars); got != days {
			t.Fatalf("the chart has %d bars, want %d", got, days)
		}
	}
}

// A custom span is two dates: the bars follow them, and a range that cannot be
// counted says so instead of drawing an empty chart that looks like "no focus".
func TestStatsCustomSpanFollowsItsDates(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteStats
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if err := session.ClickLabel(zh(i18n.MsgStatsRangeCustom)); err != nil {
		t.Fatalf("switch to a custom span: %v", err)
	}
	settle(t, session)
	if fixture.state.statsRange != statsRangeCustom {
		t.Fatalf("the range is %d, want the custom one", fixture.state.statsRange)
	}
	// The two fields start on the last week, so the span is a real one from the start.
	if got := len(fixture.state.statsBars); got != statsRangeWeek {
		t.Fatalf("the custom span starts with %d days, want %d", got, statsRangeWeek)
	}

	// Coming from a longer span, the chart must not keep its bars: the days the two
	// fields name are the days it draws.
	if err := session.ClickLabel(zh(i18n.MsgStatsRangeDays, statsRangeMonth)); err != nil {
		t.Fatalf("switch to %d days: %v", statsRangeMonth, err)
	}
	settle(t, session)
	if got := len(fixture.state.statsBars); got != statsRangeMonth {
		t.Fatalf("the month span drew %d bars", got)
	}
	if err := session.ClickLabel(zh(i18n.MsgStatsRangeCustom)); err != nil {
		t.Fatalf("switch back to a custom span: %v", err)
	}
	settle(t, session)
	if got := len(fixture.state.statsBars); got != statsRangeWeek {
		t.Fatalf("the custom span drew %d bars after a month, want %d", got, statsRangeWeek)
	}

	today := fixture.state.today()
	fixture.state.statsFromText = format.Date(i18n.ZhCN, today.AddDays(-19))
	fixture.state.statsToText = format.Date(i18n.ZhCN, today)
	settle(t, session)
	if got := len(fixture.state.statsBars); got != 20 {
		t.Fatalf("a 20-day span drew %d bars", got)
	}
	if got, want := fixture.state.statsBars[0].day, today.AddDays(-19); !got.Equal(want) {
		t.Fatalf("the first bar is %v, want %v", got, want)
	}

	// The end before the start is refused with a sentence, not with a blank chart.
	fixture.state.statsFromText = format.Date(i18n.ZhCN, today)
	fixture.state.statsToText = format.Date(i18n.ZhCN, today.AddDays(-3))
	settle(t, session)
	if fixture.state.statsValid {
		t.Fatal("a reversed span was counted")
	}
	if !session.HasText(zh(i18n.MsgStatsRangeInvalid)) {
		t.Errorf("a reversed span says nothing: %v", session.Texts())
	}
}
