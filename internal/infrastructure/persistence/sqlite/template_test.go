package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

func TestTemplateRoundTripWithTask(t *testing.T) {
	db := openFile(t)
	now := clock().Now()
	tpl := sampleTemplate(t, "0000000000TMPL0000000001", "晨间", now)
	tpl.Rule = template.TriggerRule{
		Kind:     template.TriggerWeekdays,
		Weekdays: []time.Weekday{time.Monday, time.Wednesday},
	}
	tpl.Enabled = true
	if err := db.Templates().Save(tpl); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := db.Templates().Get(tpl.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "晨间" || !got.Enabled {
		t.Fatalf("template = %+v", got)
	}
	if got.Rule.Kind != template.TriggerWeekdays || len(got.Rule.Weekdays) != 2 {
		t.Fatalf("rule = %+v", got.Rule)
	}
	if got.Rule.Weekdays[0] != time.Monday || got.Rule.Weekdays[1] != time.Wednesday {
		t.Fatalf("weekdays = %v", got.Rule.Weekdays)
	}
	if got.Task.Title != "写日记" || got.Task.Estimate != 1 {
		t.Fatalf("task = %+v", got.Task)
	}
	// The length of one pomodoro lives on the template row, so it must survive the
	// same round trip as the rest of the task it writes (migration v5).
	if got.Task.FocusMinutes != 30 {
		t.Fatalf("focus minutes = %d, want the stored 30", got.Task.FocusMinutes)
	}
	if len(got.Task.Tags) != 1 || got.Task.Tags[0] != "晨间" {
		t.Fatalf("task tags = %v", got.Task.Tags)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v / %v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestTemplateSaveReplacesTask(t *testing.T) {
	// Editing a template replaces the whole task: a leftover field would keep
	// generating a todo the user already rewrote.
	db := openFile(t)
	clk := clock()
	tpl := sampleTemplate(t, "0000000000TMPL0000000002", "晨间", clk.Now())
	if err := tpl.SetTask(template.Task{
		Title: "旧标题", Notes: "旧备注", Priority: 4, Estimate: 3, DueOffsetDays: 2,
		Tags: []string{"old"},
	}, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.Templates().Save(tpl); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if err := tpl.SetTask(template.Task{Title: "只有一项", Tags: []string{"home"}}, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.Templates().Save(tpl); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err := db.Templates().Get(tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.Title != "只有一项" || got.Task.Notes != "" || got.Task.Priority != 0 ||
		got.Task.Estimate != 0 || got.Task.DueOffsetDays != 0 || got.Task.FocusMinutes != 0 {
		t.Fatalf("a replaced task must not keep old fields: %+v", got.Task)
	}
	if len(got.Task.Tags) != 1 || got.Task.Tags[0] != "home" {
		t.Fatalf("task tags = %v", got.Task.Tags)
	}
	if !got.CreatedAt.Equal(tpl.CreatedAt) {
		t.Fatalf("CreatedAt must be preserved: %v vs %v", got.CreatedAt, tpl.CreatedAt)
	}
}

func TestTemplateListFiltersEnabled(t *testing.T) {
	db := openFile(t)
	clk := clock()
	first := sampleTemplate(t, "0000000000TMPL0000000011", "启用", clk.Now())
	if err := db.Templates().Save(first); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	second := sampleTemplate(t, "0000000000TMPL0000000012", "停用", clk.Now())
	second.Enabled = false
	if err := db.Templates().Save(second); err != nil {
		t.Fatal(err)
	}

	all, err := db.Templates().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != first.ID {
		t.Fatalf("List must be oldest first: %+v", all)
	}
	for _, tpl := range all {
		if tpl.Task.Title != "写日记" {
			t.Fatalf("List must attach the task: %+v", tpl.Task)
		}
	}
	enabled, err := db.Templates().ListEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 || enabled[0].ID != first.ID {
		t.Fatalf("enabled = %+v", enabled)
	}
}

func TestTemplateRuleKindsSurviveTheRoundTrip(t *testing.T) {
	db := openFile(t)
	clk := clock()
	rules := []template.TriggerRule{
		{Kind: template.TriggerDaily},
		{Kind: template.TriggerMonthly, DayOfMonth: 15},
		{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{
			shared.MustParseLocalDate("2026-12-25"), shared.MustParseLocalDate("2027-01-01"),
		}},
		{Kind: template.TriggerEveryNDays, EveryNDays: 3, Anchor: shared.MustParseLocalDate("2026-11-17")},
	}
	gen := &idGen{}
	for i, rule := range rules {
		tpl := sampleTemplate(t, gen.New(), "规则", clk.Now())
		tpl.Rule = rule
		if err := db.Templates().Save(tpl); err != nil {
			t.Fatalf("Save rule %d: %v", i, err)
		}
		got, err := db.Templates().Get(tpl.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Rule.Kind != rule.Kind {
			t.Fatalf("kind %d = %v", i, got.Rule.Kind)
		}
		switch rule.Kind {
		case template.TriggerMonthly:
			if got.Rule.DayOfMonth != 15 {
				t.Fatalf("day of month = %d", got.Rule.DayOfMonth)
			}
		case template.TriggerSpecificDates:
			if len(got.Rule.Dates) != 2 || !got.Rule.Dates[0].Equal(rule.Dates[0]) {
				t.Fatalf("dates = %v", got.Rule.Dates)
			}
		case template.TriggerEveryNDays:
			if got.Rule.EveryNDays != 3 || !got.Rule.Anchor.Equal(rule.Anchor) {
				t.Fatalf("every n = %d anchor = %v", got.Rule.EveryNDays, got.Rule.Anchor)
			}
		}
		// The rule must still decide the same days.
		day := shared.MustParseLocalDate("2026-11-20")
		if got.Rule.Matches(day) != rule.Matches(day) {
			t.Fatalf("rule %d changed meaning for %v", i, day)
		}
	}
}

func TestTemplateExpansionLog(t *testing.T) {
	db := openFile(t)
	clk := clock()
	tpl := sampleTemplate(t, "0000000000TMPL0000000021", "晨间", clk.Now())
	if err := db.Templates().Save(tpl); err != nil {
		t.Fatal(err)
	}
	day := shared.DateOf(clk.Now())

	logs, err := db.Templates().RecentLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("no log expected yet, got %+v", logs)
	}

	clk.Advance(2 * time.Minute)
	entry := template.LogEntry{Day: day, Created: 2, Skipped: 1, Failed: 1, At: clk.Now()}
	if err := db.Templates().MarkExpanded(tpl.ID, day, entry); err != nil {
		t.Fatalf("MarkExpanded: %v", err)
	}
	// The template's updated_at moves, its body does not.
	stamped, err := db.Templates().Get(tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stamped.UpdatedAt.Equal(clk.Now()) {
		t.Fatalf("updated at = %v", stamped.UpdatedAt)
	}
	if stamped.Task.Title != "写日记" {
		t.Fatalf("MarkExpanded must not touch the task: %+v", stamped.Task)
	}

	logs, err = db.Templates().RecentLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %+v", logs)
	}
	got := logs[0]
	if got.TemplateID != tpl.ID || got.Created != 2 || got.Skipped != 1 || got.Failed != 1 {
		t.Fatalf("log = %+v", got)
	}
	if !got.Day.Equal(day) || !got.At.Equal(clk.Now()) {
		t.Fatalf("log time = %v / %v", got.Day, got.At)
	}

	// A second day produces a second log row; LIMIT keeps the newest.
	clk.Advance(24 * time.Hour)
	nextDay := day.AddDays(1)
	if err := db.Templates().MarkExpanded(tpl.ID, nextDay, template.LogEntry{Created: 1, At: clk.Now()}); err != nil {
		t.Fatal(err)
	}
	logs, err = db.Templates().RecentLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || !logs[0].Day.Equal(nextDay) {
		t.Fatalf("newest log = %+v", logs)
	}

	if err := db.Templates().MarkExpanded("0000000000TMPL0000000099", day, template.LogEntry{At: clk.Now()}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("marking a missing template must be not-found, got %v", err)
	}
}

func TestTemplateDeleteCascades(t *testing.T) {
	db := openFile(t)
	clk := clock()
	tpl := sampleTemplate(t, "0000000000TMPL0000000031", "晨间", clk.Now())
	if err := db.Templates().Save(tpl); err != nil {
		t.Fatal(err)
	}
	if err := db.Templates().MarkExpanded(tpl.ID, shared.DateOf(clk.Now()), template.LogEntry{At: clk.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.Templates().Delete(tpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.Templates().Get(tpl.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a deleted template must be not-found, got %v", err)
	}
	if err := db.Templates().Delete(tpl.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}
	logs, err := db.Templates().RecentLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("the log must cascade with the template: %+v", logs)
	}
}

func TestSettingsStore(t *testing.T) {
	db := openFile(t)
	if _, err := db.Settings().Get("language"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing key must be ErrNotFound, got %v", err)
	}
	if err := db.Settings().Set("language", "en"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, err := db.Settings().Get("language"); err != nil || got != "en" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	// Setting the same key again replaces the value: there is one row per key, not
	// an append-only history.
	if err := db.Settings().Set("language", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Settings().Get("language"); got != "zh-CN" {
		t.Fatalf("Get after overwrite = %q", got)
	}
}
