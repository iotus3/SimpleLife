package presentation

import (
	"testing"

	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// The dashboard is a summary: it hands over to the page that owns the editing
// instead of opening a dialog of its own, which used to leave a draft nothing drew.
func TestHomeHandsOverInsteadOfOpeningEditors(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteHome
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if session.HasText(zh(i18n.MsgTaskNew)) {
		t.Errorf("the dashboard still offers to create a task: %v", session.Texts())
	}
	if session.HasText(zh(i18n.MsgLedgerNewEntry)) {
		t.Errorf("the dashboard still offers to record an entry: %v", session.Texts())
	}

	// The money card hands over to the ledger, which draws that editor itself.
	if err := session.ClickLabel(zh(i18n.MsgHomeSeeEntries)); err != nil {
		t.Fatalf("the money card has no hand-over: %v", err)
	}
	settle(t, session)
	if fixture.state.route != RouteLedger {
		t.Fatalf("route is %v after the hand-over, want the ledger", fixture.state.route)
	}
}

// The calendar a date field opens writes the day it is given: it is the same value
// the keyboard edits, so neither way of setting a date is a second store of it.
func TestDateFieldCalendarPicksADay(t *testing.T) {
	fixture := newFixture(t)
	// The in-memory store starts empty: an entry needs an account and a category,
	// which the seeded database would have.
	account, err := fixture.deps.Ledger.CreateAccount("现金", ledger.AccountCash, shared.NewMoney(0, shared.CNY))
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	category, err := fixture.deps.Ledger.CreateCategory("餐饮", ledger.CategoryExpense, "")
	if err != nil {
		t.Fatalf("category: %v", err)
	}

	session := fixture.openWindow(t)
	fixture.state.route = RouteLedger
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if err := session.ClickLabel(zh(i18n.MsgLedgerNewEntry)); err != nil {
		t.Fatalf("open the entry editor: %v", err)
	}
	settle(t, session)

	// The button is named after the field it belongs to, so the two fields of the
	// export wizard cannot be confused for each other.
	if err := session.ClickLabel(zh(i18n.MsgDatePick, zh(i18n.MsgDate))); err != nil {
		t.Fatalf("open the calendar: %v", err)
	}
	settle(t, session)
	draft := fixture.state.entryDraft
	draft.account, draft.category = account.ID, category.ID
	if !fixture.state.datePick.open(dateFieldEntry) {
		t.Fatalf("the calendar did not open; pick is %+v", fixture.state.datePick)
	}

	today := fixture.state.today()
	chosen := today.AddDays(-3)
	if !session.HasText(format.MonthLabel(i18n.ZhCN, chosen.AsMonth())) {
		t.Fatalf("the calendar shows no month title; texts: %v", session.Texts())
	}
	if err := session.ClickLabel(format.Date(i18n.ZhCN, chosen)); err != nil {
		t.Fatalf("click day %s: %v", chosen, err)
	}
	settle(t, session)
	if got, want := draft.date, format.Date(i18n.ZhCN, chosen); got != want {
		t.Fatalf("the field holds %q, want %q", got, want)
	}
	if fixture.state.datePick.open(dateFieldEntry) {
		t.Error("the calendar stayed open after a day was picked")
	}
	// The picked date is what the entry is stored with, which is the point of the
	// calendar: it writes the same field the keyboard writes.
	draft.amount = "12.00"
	if err := fixture.state.saveEntry(); err != nil {
		t.Fatalf("the picked date does not save: %v", err)
	}
	entries, err := fixture.deps.Ledger.List(ledgerapp.MonthFilter(fixture.state.ledgerMonth))
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if got := shared.DateOf(entries[0].OccurredAt); !got.Equal(chosen) {
		t.Fatalf("the entry is dated %v, want %v", got, chosen)
	}
}

// Today and Clear are the two ends of the same control: one writes today's date, the
// other leaves the field empty, which is how a task says it has no due date.
func TestDateFieldTodayAndClear(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if err := session.ClickLabel(zh(i18n.MsgTaskNew)); err != nil {
		t.Fatalf("open the task editor: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgDatePick, zh(i18n.MsgTaskFieldDue))); err != nil {
		t.Fatalf("open the calendar: %v", err)
	}
	settle(t, session)

	// Move a month back and find the month title: the arrows change what is shown
	// before a day is picked.
	month := fixture.state.datePick.month
	if err := session.ClickLabel(zh(i18n.MsgDateMonthPrev)); err != nil {
		t.Fatalf("previous month: %v", err)
	}
	settle(t, session)
	if got := fixture.state.datePick.month; got == month {
		t.Fatalf("the previous-month arrow did not move the month: %+v", got)
	}
	if !session.HasText(format.MonthLabel(i18n.ZhCN, month.AddMonths(-1))) {
		t.Fatalf("the calendar still shows %v; texts: %v", month, session.Texts())
	}

	// Today clears the month back to the running one and writes its date.
	if err := session.ClickLabel(zh(i18n.MsgToday)); err != nil {
		t.Fatalf("today: %v", err)
	}
	settle(t, session)
	if got, want := fixture.state.taskDraft.due, format.Date(i18n.ZhCN, fixture.state.today()); got != want {
		t.Fatalf("the field holds %q, want %q", got, want)
	}
	if err := session.ClickLabel(zh(i18n.MsgDatePick, zh(i18n.MsgTaskFieldDue))); err != nil {
		t.Fatalf("reopen the calendar: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgDateClear)); err != nil {
		t.Fatalf("clear: %v", err)
	}
	settle(t, session)
	if fixture.state.taskDraft.due != "" {
		t.Fatalf("clearing left %q in the field", fixture.state.taskDraft.due)
	}
	// A task with no due date is a task filed in no day list: the field cleared to
	// nothing has to save, or "undated" would be a state the editor cannot express.
	fixture.state.taskDraft.title = "没有日期的待办"
	if err := fixture.state.saveTask(); err != nil {
		t.Fatalf("a task with no due date must save: %v", err)
	}
}
