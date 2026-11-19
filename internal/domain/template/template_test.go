package template_test

import (
	"strings"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func day(s string) shared.LocalDate { return shared.MustParseLocalDate(s) }

func TestDailyMatchesEveryDay(t *testing.T) {
	r := template.TriggerRule{Kind: template.TriggerDaily}
	for _, d := range []string{"2026-11-17", "2026-02-28", "2024-02-29", "2027-01-01"} {
		if !r.Matches(day(d)) {
			t.Fatalf("daily must match %s", d)
		}
	}
	if r.Matches(shared.LocalDate{}) {
		t.Fatal("the zero date must never match")
	}
}

func TestWeekdaysMatches(t *testing.T) {
	r := template.TriggerRule{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Monday, time.Friday}}
	if !r.Matches(day("2026-11-16")) { // Monday
		t.Fatal("Monday must match")
	}
	if !r.Matches(day("2026-11-20")) { // Friday
		t.Fatal("Friday must match")
	}
	if r.Matches(day("2026-11-17")) { // Tuesday
		t.Fatal("Tuesday must not match")
	}
	empty := template.TriggerRule{Kind: template.TriggerWeekdays}
	if empty.Matches(day("2026-11-16")) {
		t.Fatal("an empty weekday list must never match")
	}
}

func TestMonthlyClampsToTheEndOfShortMonths(t *testing.T) {
	r := template.TriggerRule{Kind: template.TriggerMonthly, DayOfMonth: 31}
	cases := map[string]bool{
		"2026-01-31": true,
		"2026-01-30": false,
		"2026-02-28": true,  // February clamps to the 28th, otherwise the rule
		"2026-02-27": false, // would silently skip two months a year
		"2026-04-30": true,
		"2026-04-29": false,
		"2024-02-29": true, // leap year
		"2026-11-30": true,
	}
	for d, want := range cases {
		if got := r.Matches(day(d)); got != want {
			t.Fatalf("%s: got %v, want %v", d, got, want)
		}
	}
	if (template.TriggerRule{Kind: template.TriggerMonthly}).Matches(day("2026-11-17")) {
		t.Fatal("a zero day of month must never match")
	}
}

func TestSpecificDates(t *testing.T) {
	r := template.TriggerRule{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{day("2026-12-25"), day("2027-01-01")}}
	if !r.Matches(day("2026-12-25")) || !r.Matches(day("2027-01-01")) {
		t.Fatal("listed dates must match")
	}
	if r.Matches(day("2026-12-24")) {
		t.Fatal("an unlisted date must not match")
	}
	if (template.TriggerRule{Kind: template.TriggerSpecificDates}).Matches(day("2026-12-25")) {
		t.Fatal("an empty date list must never match")
	}
}

func TestEveryNDays(t *testing.T) {
	r := template.TriggerRule{Kind: template.TriggerEveryNDays, EveryNDays: 3, Anchor: day("2026-11-17")}
	for _, c := range []struct {
		d    string
		want bool
	}{
		{"2026-11-16", false}, // before the anchor
		{"2026-11-17", true},  // the anchor itself
		{"2026-11-18", false},
		{"2026-11-20", true},
		{"2026-11-23", true},
	} {
		if got := r.Matches(day(c.d)); got != c.want {
			t.Fatalf("%s: got %v, want %v", c.d, got, c.want)
		}
	}
	if (template.TriggerRule{Kind: template.TriggerEveryNDays, EveryNDays: 1, Anchor: day("2026-11-17")}).Matches(day("2026-11-17")) {
		t.Fatal("an out-of-range interval must never match")
	}
	if (template.TriggerRule{Kind: template.TriggerEveryNDays, EveryNDays: 3}).Matches(day("2026-11-17")) {
		t.Fatal("a missing anchor must never match")
	}
}

func TestUnknownKindNeverMatches(t *testing.T) {
	if (template.TriggerRule{Kind: template.TriggerKind(99)}).Matches(day("2026-11-17")) {
		t.Fatal("an unknown kind must never match")
	}
}

func TestTriggerKindRoundTrip(t *testing.T) {
	kinds := []template.TriggerKind{
		template.TriggerDaily, template.TriggerWeekdays, template.TriggerMonthly,
		template.TriggerSpecificDates, template.TriggerEveryNDays,
	}
	for _, k := range kinds {
		back, err := template.ParseTriggerKind(k.String())
		if err != nil || back != k {
			t.Fatalf("round trip failed for %v: %v %v", k, back, err)
		}
	}
	if _, err := template.ParseTriggerKind("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown kind must be invalid, got %v", err)
	}
	if got := template.TriggerKind(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
}

func TestTriggerRuleValidate(t *testing.T) {
	valid := []template.TriggerRule{
		{Kind: template.TriggerDaily},
		{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Monday}},
		{Kind: template.TriggerMonthly, DayOfMonth: 15},
		{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{day("2026-12-25")}},
		{Kind: template.TriggerEveryNDays, EveryNDays: 7, Anchor: day("2026-11-17")},
	}
	for i, r := range valid {
		if err := r.Validate(); err != nil {
			t.Fatalf("valid rule %d rejected: %v", i, err)
		}
	}
	invalid := []template.TriggerRule{
		{Kind: template.TriggerWeekdays},
		{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Weekday(9)}},
		{Kind: template.TriggerMonthly},
		{Kind: template.TriggerMonthly, DayOfMonth: 32},
		{Kind: template.TriggerSpecificDates},
		{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{{}}},
		{Kind: template.TriggerEveryNDays, EveryNDays: 1, Anchor: day("2026-11-17")},
		{Kind: template.TriggerEveryNDays, EveryNDays: 366, Anchor: day("2026-11-17")},
		{Kind: template.TriggerEveryNDays, EveryNDays: 7},
		{Kind: template.TriggerKind(99)},
	}
	for i, r := range invalid {
		if err := r.Validate(); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("invalid rule %d accepted", i)
		}
	}
}

func TestTemplateValidateAndSetItems(t *testing.T) {
	now := at("2026-11-17 09:00:00")
	tpl, err := template.New("01J0TEMPLATE000000000000C", "  晨间  ", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tpl.Name != "晨间" {
		t.Fatalf("name must be trimmed, got %q", tpl.Name)
	}
	if tpl.Enabled {
		t.Fatal("a new template must start disabled so a half-made one cannot fire")
	}
	if tpl.Rule.Kind != template.TriggerDaily {
		t.Fatalf("default rule = %v", tpl.Rule.Kind)
	}
	if _, err := template.New("01J0TEMPLATE000000000000C", "  ", now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("blank name must be invalid, got %v", err)
	}
	if _, err := template.New("", "x", now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty id must be invalid, got %v", err)
	}

	task := template.Task{Title: "写周报", Priority: 2, Estimate: 3, Tags: []string{"work"}}
	if err := tpl.SetTask(task, now); err != nil {
		t.Fatalf("SetTask: %v", err)
	}
	if tpl.Task.Title != "写周报" || tpl.Task.Estimate != 3 {
		t.Fatalf("SetTask must store the whole task: %+v", tpl.Task)
	}
	if !tpl.UpdatedAt.Equal(now) {
		t.Fatal("SetTask must stamp UpdatedAt")
	}
	if err := tpl.SetTask(template.Task{Title: "  "}, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a blank task title must be refused, got %v", err)
	}
	if !shared.IsKind((&template.Template{ID: "01J0TEMPLATE000000000000C", Name: "x"}).Validate(), shared.KindInvalid) {
		t.Fatal("an invalid rule must fail validation")
	}
}

func TestTaskValidate(t *testing.T) {
	good := template.Task{Title: "x", Priority: 4, Estimate: 3, DueOffsetDays: 2}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
	bad := []template.Task{
		{Title: "  "},
		{Title: "x", Priority: 5},
		{Title: "x", Priority: -1},
		{Title: "x", Estimate: -1},
		{Title: "x", DueOffsetDays: -1},
		{Title: "x", DueOffsetDays: 366},
		{Title: "x", FocusMinutes: -1},
		{Title: "x", FocusMinutes: shared.FocusMinutesMax + 1},
		{Title: strings.Repeat("x", template.TitleMaxRunes+1)},
	}
	for i, task := range bad {
		if err := task.Validate(); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("bad task %d accepted", i)
		}
	}
}

// A template may carry the length of one pomodoro for the task it writes, and zero
// means "no length of its own". The bounds are shared with the task aggregate, so a
// template can never save a number the task it produces would have to refuse.
func TestTemplateTaskCarriesAPomodoroLength(t *testing.T) {
	for _, minutes := range []int{0, shared.FocusMinutesMin, 25, shared.FocusMinutesMax} {
		value := template.Task{Title: "x", FocusMinutes: minutes}
		if err := value.Validate(); err != nil {
			t.Fatalf("length %d rejected: %v", minutes, err)
		}
	}
	// The bounds come from domain/shared, so the two aggregates cannot drift apart. Zero is
	// legal, which is why the out-of-range values start below one minute.
	for _, minutes := range []int{-1, shared.FocusMinutesMax + 1} {
		value := template.Task{Title: "x", FocusMinutes: minutes}
		if err := value.Validate(); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("length %d accepted", minutes)
		}
	}
}

func TestDisableStopsFutureExpansion(t *testing.T) {
	now := at("2026-11-17 09:00:00")
	tpl, _ := template.New("01J0TEMPLATE000000000000C", "晨间", now)
	tpl.Enabled = true
	later := now.Add(time.Hour)
	tpl.Disable(later)
	if tpl.Enabled {
		t.Fatal("Disable must clear Enabled")
	}
	if !tpl.UpdatedAt.Equal(later) {
		t.Fatal("Disable must stamp UpdatedAt")
	}
}
