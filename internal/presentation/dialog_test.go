package presentation

import (
	"testing"

	lapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// These tests drive the dialogs through the window: a click opens the dialog, the
// text fields are typed into, Save is clicked where it is drawn, and the effect is
// read back through the application services.

func TestTemplateFromTheDialogCreatesTodayTasks(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTemplates
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }

	if err := session.ClickLabel(zh(i18n.MsgTemplateNew)); err != nil {
		t.Fatalf("open the editor: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgTemplateFieldName)); err != nil {
		t.Fatalf("click the name field: %v", err)
	}
	if err := session.Type("晨间"); err != nil {
		t.Fatalf("type the name: %v", err)
	}
	if err := session.ClickLabel(zh(i18n.MsgTemplateFieldTitle)); err != nil {
		t.Fatalf("click the task field: %v", err)
	}
	if err := session.Type("写日记"); err != nil {
		t.Fatalf("type the task: %v", err)
	}
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save the template: %v", err)
	}
	settle(t, session)

	templates, err := fixture.deps.Templates.List()
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	if templates[0].Name != "晨间" {
		t.Errorf("template name is %q, want 晨间", templates[0].Name)
	}
	if templates[0].Task.Title != "写日记" {
		t.Fatalf("template task is %+v, want one titled 写日记", templates[0].Task)
	}

	// The daily rule fires the same day: expanding creates today's task.
	today := fixture.state.today()
	result, err := fixture.deps.Templates.Expand(today)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("expand created %d tasks, want 1", result.Created)
	}
	list, err := fixture.deps.Tasks.Today(today)
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	if len(list) != 1 || list[0].Title != "写日记" {
		t.Fatalf("today's tasks are %+v, want the template's item", titles(list))
	}
	if list[0].OriginTemplateID.IsZero() {
		t.Error("the generated task does not point back at its template")
	}
}

func TestTemplateDialogKeepsTheDialogOpenOnBadInput(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTemplates
	fixture.state.markAll()
	settle(t, session)

	if err := session.ClickLabel(i18n.T(i18n.ZhCN, i18n.MsgTemplateNew)); err != nil {
		t.Fatalf("open the editor: %v", err)
	}
	settle(t, session)
	// No name and no item title: saving must report the failure and leave the
	// dialog open, so the user can correct it.
	if err := session.ClickLabel(i18n.T(i18n.ZhCN, i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	templates, err := fixture.deps.Templates.List()
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(templates) != 0 {
		t.Fatalf("an empty template was saved: %+v", templates)
	}
	if draft := fixture.state.templateDraft; draft == nil || !draft.open {
		t.Error("the dialog closed on a failed save")
	}
}

func TestLedgerEntryFromTheDialogIsRecorded(t *testing.T) {
	fixture := newFixture(t)
	// The application seeds an account and a category on first run; the in-memory
	// fixture does not, so the test makes the two rows an entry needs.
	if _, err := fixture.deps.Ledger.CreateAccount("Wallet", ledger.AccountCash, shared.ZeroMoney(shared.CNY)); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := fixture.deps.Ledger.CreateCategory("饮食", ledger.CategoryExpense, ""); err != nil {
		t.Fatalf("create category: %v", err)
	}
	session := fixture.openWindow(t)
	fixture.state.route = RouteLedger
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if err := session.ClickLabel(zh(i18n.MsgLedgerNewEntry)); err != nil {
		t.Fatalf("open the entry dialog: %v", err)
	}
	settle(t, session)
	if !fixture.state.entryDraft.open {
		t.Fatal("the entry dialog did not open")
	}
	if fixture.state.entryDraft.account.IsZero() {
		t.Fatal("the dialog did not pick the only account")
	}
	if err := session.ClickLabel(zh(i18n.MsgAmount)); err != nil {
		t.Fatalf("click the amount field: %v", err)
	}
	if err := session.Type("12.30"); err != nil {
		t.Fatalf("type the amount: %v", err)
	}
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save the entry: %v", err)
	}
	settle(t, session)

	entries, err := fixture.deps.Ledger.List(lapp.MonthFilter(fixture.state.ledgerMonth))
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Amount.Minor() != 1230 {
		t.Errorf("amount is %d minor units, want 1230", entry.Amount.Minor())
	}
	if entry.Kind != fixture.state.entryDraft.kind {
		t.Errorf("entry kind is %v, want the draft's %v", entry.Kind, fixture.state.entryDraft.kind)
	}
	if entry.AccountID.IsZero() {
		t.Error("the entry has no account")
	}
}

func TestEntryDialogKeepsTheDialogOpenOnABadAmount(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.deps.Ledger.CreateAccount("Wallet", ledger.AccountCash, shared.ZeroMoney(shared.CNY)); err != nil {
		t.Fatalf("create account: %v", err)
	}
	session := fixture.openWindow(t)
	fixture.state.route = RouteLedger
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if err := session.ClickLabel(zh(i18n.MsgLedgerNewEntry)); err != nil {
		t.Fatalf("open the entry dialog: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgAmount)); err != nil {
		t.Fatalf("click the amount field: %v", err)
	}
	if err := session.Type("twelve"); err != nil {
		t.Fatalf("type the amount: %v", err)
	}
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	entries, err := fixture.deps.Ledger.List(lapp.MonthFilter(fixture.state.ledgerMonth))
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("an unreadable amount was stored: %+v", entries)
	}
	if draft := fixture.state.entryDraft; draft == nil || !draft.open {
		t.Error("the dialog closed on a rejected amount, losing what was typed")
	}
}

func TestTaskDialogCreatesATaskForTheChosenDay(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if err := session.ClickLabel(zh(i18n.MsgTaskNew)); err != nil {
		t.Fatalf("open the task editor: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgTaskFieldTitle)); err != nil {
		t.Fatalf("click the title field: %v", err)
	}
	if err := session.Type("季度复盘"); err != nil {
		t.Fatalf("type the title: %v", err)
	}
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save the task: %v", err)
	}
	settle(t, session)

	list, err := fixture.deps.Tasks.Today(fixture.state.today())
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	found := false
	for _, value := range list {
		if value.Title == "季度复盘" {
			found = true
		}
	}
	if !found {
		t.Fatalf("today's tasks are %v, want the new one", titles(list))
	}
}

// titles lists the titles of a set of tasks, for failure messages.
func titles(list []*task.Task) []string {
	out := make([]string, 0, len(list))
	for _, value := range list {
		out = append(out, value.Title)
	}
	return out
}
