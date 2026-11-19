package presentation

import (
	"testing"

	"github.com/vivaui/viva/headless"

	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// openLedgerTab puts the ledger page on one of its tabs and paints it.
func openLedgerTab(t *testing.T, fixture *testFixture, tab int) *headless.Session {
	t.Helper()
	session := fixture.openWindow(t)
	fixture.state.route = RouteLedger
	fixture.state.ledgerTab = tab
	fixture.state.markAll()
	settle(t, session)
	return session
}

// TestAccountRowIsEditableAndDeletable is requirement 3 of the ninth round: an account used to be
// creatable and then frozen. The name is set on the draft rather than typed, because replacing a
// value from a script tests the text editor, not the account editor.
func TestAccountRowIsEditableAndDeletable(t *testing.T) {
	fixture := newFixture(t)
	wallet, err := fixture.deps.Ledger.CreateAccount("Old name", ledger.AccountCash, shared.ZeroMoney(shared.CNY))
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	session := openLedgerTab(t, fixture, 2)
	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }

	if err := session.ClickLabel(zh(i18n.MsgLedgerAccountEdit)); err != nil {
		t.Fatalf("open the account editor: %v", err)
	}
	settle(t, session)
	draft := fixture.state.accountDraft
	if draft == nil || !draft.open {
		t.Fatal("the account editor did not open")
	}
	if draft.id != wallet.ID {
		t.Fatalf("the editor is bound to %q, want %q", draft.id, wallet.ID)
	}
	if draft.name != "Old name" {
		t.Fatalf("the editor shows %q, want the stored name", draft.name)
	}
	draft.name = "Wallet"
	draft.kind = ledger.AccountDebitCard
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	renamed, err := fixture.deps.Ledger.Account(wallet.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if renamed.Name != "Wallet" {
		t.Fatalf("name is %q, want Wallet", renamed.Name)
	}
	if renamed.Kind != ledger.AccountDebitCard {
		t.Fatalf("kind is %v, want the edited one", renamed.Kind)
	}
	if !session.HasText("Wallet") {
		t.Errorf("the row still shows the old name: %v", session.Texts())
	}

	// Deleting asks first, then removes the row: an account with no entries has
	// nothing to keep it alive.
	if err := session.ClickLabel(zh(i18n.MsgDelete)); err != nil {
		t.Fatalf("ask to delete: %v", err)
	}
	settle(t, session)
	if !fixture.state.confirm.open {
		t.Fatal("deleting an account did not ask first")
	}
	if err := session.ClickLabel(zh(i18n.MsgDelete)); err != nil {
		t.Fatalf("confirm the delete: %v", err)
	}
	settle(t, session)
	if _, err := fixture.deps.Ledger.Account(wallet.ID); err == nil {
		t.Fatal("the account survived the delete")
	}
}

// TestBuiltinAccountCannotBeDeleted keeps the seeding honest: a seeded row that can
// be deleted comes back on the next launch, which reads as a bug.
func TestBuiltinAccountCannotBeDeleted(t *testing.T) {
	fixture := newFixture(t)
	cash, err := fixture.deps.Ledger.CreateAccount("Cash", ledger.AccountCash, shared.ZeroMoney(shared.CNY))
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	stored, err := fixture.store.Accounts().Get(cash.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	stored.BuiltinKey = ledger.BuiltinAccountCash
	if err := fixture.store.Accounts().Update(stored); err != nil {
		t.Fatalf("mark builtin: %v", err)
	}
	session := openLedgerTab(t, fixture, 2)

	if !session.HasText(i18n.T(i18n.ZhCN, i18n.MsgLedgerCategoryBuiltin)) {
		t.Errorf("the row is not marked as built-in: %v", session.Texts())
	}
	if err := fixture.deps.Ledger.DeleteAccount(cash.ID); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("deleting a builtin account must be refused, got %v", err)
	}
	if _, err := fixture.deps.Ledger.Account(cash.ID); err != nil {
		t.Fatalf("the refusal still deleted the row: %v", err)
	}
}

// TestBudgetAppearsBeforeAnythingIsSpent is requirement 4: a ceiling used to be
// listed only after a purchase, because the list was built from the spending. Setting
// a budget and watching nothing appear is what made the button look broken.
func TestBudgetAppearsBeforeAnythingIsSpent(t *testing.T) {
	fixture := newFixture(t)
	dining, err := fixture.deps.Ledger.CreateCategory("Dining", ledger.CategoryExpense, "")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	session := openLedgerTab(t, fixture, 3)
	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }

	// Nothing has been spent, so the tab starts empty.
	if !session.HasText(zh(i18n.MsgLedgerBudgetEmpty)) {
		t.Fatalf("the empty budget tab does not say it is empty: %v", session.Texts())
	}

	if err := session.ClickLabel(zh(i18n.MsgLedgerBudgetNew)); err != nil {
		t.Fatalf("open the budget editor: %v", err)
	}
	settle(t, session)
	if draft := fixture.state.budgetDraft; draft == nil || !draft.open || draft.editing {
		t.Fatal("the budget editor did not open in create mode")
	}
	fixture.state.budgetDraft.category = dining.ID
	fixture.state.budgetDraft.limit = "300"
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save the budget: %v", err)
	}
	settle(t, session)

	budgets, err := fixture.deps.Ledger.Budgets(fixture.state.ledgerMonth)
	if err != nil {
		t.Fatalf("list budgets: %v", err)
	}
	if len(budgets) != 1 || budgets[0].Limit.Minor() != 30000 {
		t.Fatalf("budgets = %+v, want one ceiling of 300.00", budgets)
	}
	if !session.HasText("Dining") {
		t.Errorf("the ceiling is not on the tab before any spending: %v", session.Texts())
	}
	if session.HasText(zh(i18n.MsgLedgerBudgetEmpty)) {
		t.Error("the tab still says it is empty with a ceiling set")
	}
}

// TestBudgetCanBeEditedAndDeleted is the rest of requirement 4: a ceiling that can
// only be created is a typo the user has to live with for the rest of the month.
func TestBudgetCanBeEditedAndDeleted(t *testing.T) {
	fixture := newFixture(t)
	dining, err := fixture.deps.Ledger.CreateCategory("Dining", ledger.CategoryExpense, "")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	budget, err := fixture.deps.Ledger.UpsertBudget(dining.ID, fixture.state.ledgerMonth,
		shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
	session := openLedgerTab(t, fixture, 3)
	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }

	if err := session.ClickLabel(zh(i18n.MsgLedgerBudgetEdit)); err != nil {
		t.Fatalf("open the budget editor: %v", err)
	}
	settle(t, session)
	draft := fixture.state.budgetDraft
	if draft == nil || !draft.editing {
		t.Fatal("the editor is not in edit mode")
	}
	if draft.category != dining.ID {
		t.Fatalf("the editor is bound to category %q, want %q", draft.category, dining.ID)
	}
	draft.limit = "250"
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	budgets, err := fixture.deps.Ledger.Budgets(fixture.state.ledgerMonth)
	if err != nil {
		t.Fatalf("list budgets: %v", err)
	}
	if len(budgets) != 1 {
		t.Fatalf("editing created a second ceiling: %+v", budgets)
	}
	if budgets[0].ID != budget.ID {
		t.Errorf("the ceiling was replaced instead of edited: %s → %s", budget.ID, budgets[0].ID)
	}
	if budgets[0].Limit.Minor() != 25000 {
		t.Errorf("limit is %d, want 25000", budgets[0].Limit.Minor())
	}

	if err := session.ClickLabel(zh(i18n.MsgDelete)); err != nil {
		t.Fatalf("ask to delete the budget: %v", err)
	}
	settle(t, session)
	if !fixture.state.confirm.open {
		t.Fatal("deleting a budget did not ask first")
	}
	if err := session.ClickLabel(zh(i18n.MsgDelete)); err != nil {
		t.Fatalf("confirm the delete: %v", err)
	}
	settle(t, session)
	budgets, err = fixture.deps.Ledger.Budgets(fixture.state.ledgerMonth)
	if err != nil {
		t.Fatalf("list budgets: %v", err)
	}
	if len(budgets) != 0 {
		t.Fatalf("the ceiling survived the delete: %+v", budgets)
	}
}
