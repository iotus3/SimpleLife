package ledger_test

import (
	"testing"
	"time"

	app "github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/contracts"
	domain "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

type fixture struct {
	svc        *app.Service
	store      *store.Store
	clock      *harness.Clock
	cash       shared.ID
	bank       shared.ID
	dining     shared.ID
	salary     shared.ID
	focusAward shared.ID
	focusFine  shared.ID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 12:00:00"))
	svc := app.New(st.Entries(), st.Accounts(), st.Categories(), st.Budgets(), st, clk, harness.NewIDs())

	cash, err := svc.CreateAccount("现金", domain.AccountCash, shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	bank, err := svc.CreateAccount("银行卡", domain.AccountDebitCard, shared.NewMoney(500000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	dining, err := svc.CreateCategory("餐饮", domain.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	salary, err := svc.CreateCategory("工资", domain.CategoryIncome, "")
	if err != nil {
		t.Fatal(err)
	}
	award, err := svc.CreateCategory("专注奖励", domain.CategoryIncome, "")
	if err != nil {
		t.Fatal(err)
	}
	fine, err := svc.CreateCategory("专注罚金", domain.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, store: st, clock: clk, cash: cash.ID, bank: bank.ID, dining: dining.ID, salary: salary.ID, focusAward: award.ID, focusFine: fine.ID}
}

func cny(minor int64) shared.Money { return shared.NewMoney(minor, shared.CNY) }

func TestCreateEntryValidatesAgainstTheStore(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(3550), AccountID: f.cash, CategoryID: f.dining,
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if e.SignedAmount() != -3550 {
		t.Fatalf("signed amount = %d", e.SignedAmount())
	}

	// Direction mismatch: a dining (expense) category on an income entry.
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryIncome, Amount: cny(100), AccountID: f.cash, CategoryID: f.dining,
	}); shared.Code(err) != "ledger.entry.category_direction" {
		t.Fatalf("code = %q", shared.Code(err))
	}

	// Currency mismatch: a USD amount on a CNY account.
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: shared.NewMoney(100, shared.USD), AccountID: f.cash, CategoryID: f.dining,
	}); shared.Code(err) != "ledger.entry.currency_mismatch" {
		t.Fatalf("code = %q", shared.Code(err))
	}

	missing := shared.ID("00000000000000000000000099")
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1), AccountID: missing, CategoryID: f.dining,
	}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing account must be not-found, got %v", err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1), AccountID: f.cash, CategoryID: missing,
	}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing category must be not-found, got %v", err)
	}
}

func TestTransferMovesMoneyWithoutCountingAsIncome(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryTransfer, Amount: cny(20000), AccountID: f.bank, ToAccountID: f.cash,
	}); err != nil {
		t.Fatalf("CreateEntry transfer: %v", err)
	}
	summary, err := f.svc.Summarise(shared.MonthOf(f.clock.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Income) != 0 || len(summary.Expense) != 0 {
		t.Fatalf("a transfer must not appear in income or expense: %+v", summary)
	}
	if summary.EntryCount != 1 {
		t.Fatalf("the transfer must still be listed, count = %d", summary.EntryCount)
	}
	balances, err := f.svc.Balances(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range balances {
		switch b.Account.ID {
		case f.bank:
			if b.Balance.Minor() != 480000 {
				t.Fatalf("bank balance = %d, want 480000", b.Balance.Minor())
			}
		case f.cash:
			if b.Balance.Minor() != 30000 {
				t.Fatalf("cash balance = %d, want 30000", b.Balance.Minor())
			}
		}
	}
}

func TestSummariseKeepsCurrenciesSeparate(t *testing.T) {
	f := newFixture(t)
	usd, err := f.svc.CreateAccount("Dollar", domain.AccountSavings, shared.NewMoney(0, shared.USD))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1000), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: shared.NewMoney(500, shared.USD), AccountID: usd.ID, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	summary, err := f.svc.Summarise(shared.MonthOf(f.clock.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Expense) != 2 {
		t.Fatalf("want one subtotal per currency, got %+v", summary.Expense)
	}
	for _, total := range summary.Expense {
		switch total.Currency {
		case shared.CNY:
			if total.Amount.Minor() != 1000 {
				t.Fatalf("CNY total = %d", total.Amount.Minor())
			}
		case shared.USD:
			if total.Amount.Minor() != 500 {
				t.Fatalf("USD total = %d", total.Amount.Minor())
			}
		default:
			t.Fatalf("unexpected currency %v", total.Currency)
		}
	}
}

func TestNetTotalsSubtractPerCurrency(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryIncome, Amount: cny(500000), AccountID: f.bank, CategoryID: f.salary,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(3550), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	summary, err := f.svc.Summarise(shared.MonthOf(f.clock.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Net) != 1 || summary.Net[0].Amount.Minor() != 500000-3550 {
		t.Fatalf("net = %+v", summary.Net)
	}
}

func TestUpdateEntryKeepsIdentity(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1000), AccountID: f.cash, CategoryID: f.dining,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.svc.UpdateEntry(e.ID, app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(2000), AccountID: f.cash, CategoryID: f.dining, Note: "改",
	})
	if err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}
	if updated.ID != e.ID || updated.Amount.Minor() != 2000 || updated.Note != "改" {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.CreatedAt.Equal(e.CreatedAt) {
		t.Fatal("CreatedAt must be preserved")
	}
	if _, err := f.svc.UpdateEntry(shared.ID("00000000000000000000000099"), app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1), AccountID: f.cash, CategoryID: f.dining,
	}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing entry must be not-found, got %v", err)
	}
}

func TestFocusBookingsAreProtected(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.Book(contracts.BookingRequest{
		Direction: contracts.BookingIncome, Amount: cny(1250),
		AccountID: f.cash, CategoryID: f.focusAward,
		SessionID: "00000000000000000000000042", OccurredAt: f.clock.Now(),
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}
	entries, err := f.svc.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Origin != domain.OriginFocusReward {
		t.Fatalf("entries = %+v", entries)
	}
	booking := entries[0]
	if err := f.svc.DeleteEntry(booking.ID); shared.Code(err) != "ledger.entry.locked" {
		t.Fatalf("a focus booking must not be deletable, got %v", err)
	}
	// Editing in place on the same day is allowed.
	if _, err := f.svc.UpdateEntry(booking.ID, app.EntryInput{
		Kind: domain.EntryIncome, Amount: cny(1000), AccountID: f.cash, CategoryID: f.focusAward, Note: "手动改",
	}); err != nil {
		t.Fatalf("same-day edit must be allowed: %v", err)
	}
	// Moving it to another day would corrupt the daily cap, so it is refused.
	f.clock.Advance(48 * time.Hour)
	if _, err := f.svc.UpdateEntry(booking.ID, app.EntryInput{
		Kind: domain.EntryIncome, Amount: cny(1000), AccountID: f.cash, CategoryID: f.focusAward, Note: "隔日改",
	}); shared.Code(err) != "ledger.entry.locked" {
		t.Fatalf("moving a booking to another day must be refused, got %v", err)
	}
}

func TestBookRejectsInvalidRequest(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.Book(contracts.BookingRequest{
		Direction: contracts.BookingExpense, Amount: cny(0),
		AccountID: f.cash, CategoryID: f.focusFine,
		SessionID: "00000000000000000000000043",
	}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a zero amount must be invalid, got %v", err)
	}
}

func TestAccountGuards(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(100), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteAccount(f.cash); shared.Code(err) != "ledger.account.in_use" {
		t.Fatalf("an account with entries must not be deleted, got %v", err)
	}
	if err := f.svc.ArchiveAccount(f.cash); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	active, err := f.svc.Accounts(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.ID == f.cash {
			t.Fatal("an archived account must not be listed")
		}
	}
	if _, err := f.svc.RenameAccount(f.cash, "钱包"); err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}
	if err := f.svc.DeleteAccount(f.bank); err != nil {
		t.Fatalf("an unused account must be deletable: %v", err)
	}
	if _, err := f.svc.CreateAccount("  ", domain.AccountCash, cny(0)); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a blank name must be invalid, got %v", err)
	}
}

func TestCategoryGuards(t *testing.T) {
	f := newFixture(t)
	parent, err := f.svc.CreateCategory("吃饭", domain.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.svc.CreateCategory("午饭", domain.CategoryExpense, parent.ID)
	if err != nil {
		t.Fatalf("CreateCategory child: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("parent = %v", child.ParentID)
	}
	if err := f.svc.DeleteCategory(parent.ID); shared.Code(err) != "ledger.category.has_children" {
		t.Fatalf("a parent with children must not be deleted, got %v", err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(100), AccountID: f.cash, CategoryID: child.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(child.ID); shared.Code(err) != "ledger.category.in_use" {
		t.Fatalf("a category in use must not be deleted, got %v", err)
	}
	if _, err := f.svc.RenameCategory(child.ID, "午餐"); err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}

	builtin, err := domain.NewCategory("00000000000000000000000070", "专注奖励", domain.CategoryIncome)
	if err != nil {
		t.Fatal(err)
	}
	builtin.BuiltinKey = domain.BuiltinCategoryFocusAward
	if err := f.store.Categories().Create(builtin); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(builtin.ID); shared.Code(err) != "ledger.category.builtin" {
		t.Fatalf("a builtin category must not be deleted, got %v", err)
	}
	if _, err := f.svc.CreateCategory("x", domain.CategoryExpense, shared.ID("00000000000000000000000099")); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing parent must be not-found, got %v", err)
	}
}

func TestBudgetLifecycle(t *testing.T) {
	f := newFixture(t)
	month := shared.MonthOf(f.clock.Now())
	b, err := f.svc.UpsertBudget(f.dining, month, cny(100000))
	if err != nil {
		t.Fatalf("UpsertBudget: %v", err)
	}
	again, err := f.svc.UpsertBudget(f.dining, month, cny(200000))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != b.ID {
		t.Fatal("upserting the same month and category must update, not duplicate")
	}
	list, err := f.svc.Budgets(month)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Limit.Minor() != 200000 {
		t.Fatalf("budgets = %+v", list)
	}
	if err := f.svc.DeleteBudget(b.ID); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	if _, err := f.svc.UpsertBudget(f.dining, month, cny(0)); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a zero limit must be invalid, got %v", err)
	}
}

func TestCategorySpendingSortsAndAttachesBudgets(t *testing.T) {
	f := newFixture(t)
	transport, err := f.svc.CreateCategory("交通", domain.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1000), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(5000), AccountID: f.cash, CategoryID: transport.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpsertBudget(transport.ID, shared.MonthOf(f.clock.Now()), cny(20000)); err != nil {
		t.Fatal(err)
	}
	spending, err := f.svc.CategorySpending(shared.MonthOf(f.clock.Now()))
	if err != nil {
		t.Fatalf("CategorySpending: %v", err)
	}
	if len(spending) != 2 {
		t.Fatalf("spending = %+v", spending)
	}
	if spending[0].Category.ID != transport.ID {
		t.Fatalf("the largest spend must come first: %+v", spending)
	}
	if spending[0].Budget == nil {
		t.Fatal("the budget must be attached")
	}
	if spending[1].Budget != nil {
		t.Fatal("a category without a budget must report nil")
	}
}

func TestRecentAndSearch(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1000), AccountID: f.cash, CategoryID: f.dining, Note: "午饭",
	}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(2000), AccountID: f.cash, CategoryID: f.dining, Note: "晚饭",
	}); err != nil {
		t.Fatal(err)
	}
	recent, err := f.svc.Recent(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Note != "晚饭" {
		t.Fatalf("recent = %+v", recent)
	}
	found, err := f.svc.Search("午饭", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("search = %+v", found)
	}
}

func TestMonthFilterCoversTheWholeMonth(t *testing.T) {
	m := shared.MustParseMonth("2026-11")
	filter := app.MonthFilter(m)
	if !filter.From.Equal(m.Start()) {
		t.Fatalf("from = %v", filter.From)
	}
	lastInstant := shared.MustParseLocalDate("2026-11-30").EndOfDay()
	if filter.To.Before(lastInstant) {
		t.Fatalf("to = %v must include the last instant of the month", filter.To)
	}
}

func TestReadAccessorsReturnTheStoredRows(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1200), AccountID: f.cash, CategoryID: f.dining,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.List(domain.EntryFilter{AccountID: f.cash, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != e.ID {
		t.Fatalf("entries = %+v", got)
	}
	acc, err := f.svc.Account(f.cash)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if acc.Name != "现金" {
		t.Fatalf("account = %+v", acc)
	}
	cat, err := f.svc.Category(f.dining)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if cat.Name != "餐饮" {
		t.Fatalf("category = %+v", cat)
	}
	all, err := f.svc.Categories()
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("categories = %d", len(all))
	}
	if _, err := f.svc.Account(shared.ID("00000000000000000000000099")); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing account must be not-found, got %v", err)
	}
}

func TestBalancesIncludeTransfersOnBothSides(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(2000), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryTransfer, Amount: cny(5000), AccountID: f.cash, ToAccountID: f.bank,
	}); err != nil {
		t.Fatal(err)
	}
	balances, err := f.svc.Balances(false)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	byID := map[shared.ID]shared.Money{}
	for _, b := range balances {
		byID[b.Account.ID] = b.Balance
	}
	// 100.00 - 20.00 spent - 50.00 moved out.
	if got := byID[f.cash].Minor(); got != 3000 {
		t.Fatalf("cash = %d, want 3000", got)
	}
	// 5000.00 + 50.00 moved in.
	if got := byID[f.bank].Minor(); got != 505000 {
		t.Fatalf("bank = %d, want 505000", got)
	}
}

func TestDeleteEntryGuards(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(500), AccountID: f.cash, CategoryID: f.dining,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteEntry(e.ID); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	if err := f.svc.DeleteEntry(e.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}

	// A booking made by a finished pomodoro cannot be removed: doing so would undo
	// the work and confuse the daily cap.
	if err := f.svc.Book(contracts.BookingRequest{
		SessionID: "00000000000000000000000060", Direction: contracts.BookingIncome,
		Amount: cny(50), AccountID: f.cash, CategoryID: f.focusAward, OccurredAt: f.clock.Now(),
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}
	booked, err := f.svc.List(domain.EntryFilter{Origin: []domain.Origin{domain.OriginFocusReward}})
	if err != nil {
		t.Fatal(err)
	}
	if len(booked) != 1 {
		t.Fatalf("bookings = %+v", booked)
	}
	if err := f.svc.DeleteEntry(booked[0].ID); shared.Code(err) != "ledger.entry.locked" {
		t.Fatalf("a focus booking must be locked, got %v", err)
	}
}

func TestAccountRenamesRejectBuiltinsAndMissingRows(t *testing.T) {
	f := newFixture(t)
	builtin := &domain.Account{
		ID: "00000000000000000000000071", Name: "专注储蓄", BuiltinKey: domain.BuiltinAccountFocusPot,
		Kind: domain.AccountSavings, Initial: cny(0),
	}
	if err := f.store.Accounts().Create(builtin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RenameAccount(builtin.ID, "我的储蓄"); shared.Code(err) != "ledger.account.builtin" {
		t.Fatalf("a builtin account must not be renamed, got %v", err)
	}
	if _, err := f.svc.RenameAccount(shared.ID("00000000000000000000000099"), "x"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("renaming a missing account must be not-found, got %v", err)
	}
	if err := f.svc.ArchiveAccount(shared.ID("00000000000000000000000099")); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("archiving a missing account must be not-found, got %v", err)
	}
	if _, err := f.svc.RenameCategory(shared.ID("00000000000000000000000099"), "x"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("renaming a missing category must be not-found, got %v", err)
	}
	if err := f.svc.DeleteCategory(shared.ID("00000000000000000000000099")); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting a missing category must be not-found, got %v", err)
	}
}

// An account can be edited (name and kind) after it exists, and a built-in one keeps
// its name because that name is a translation key.
func TestAccountUpdatesKeepWhatMayNotChange(t *testing.T) {
	f := newFixture(t)
	updated, err := f.svc.UpdateAccount(f.cash, "钱包", domain.AccountWallet)
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if updated.Name != "钱包" || updated.Kind != domain.AccountWallet {
		t.Fatalf("account = %+v", updated)
	}
	// The currency and the opening balance are what the entries were booked in, so an
	// edit must leave them exactly as they were.
	if updated.Currency() != shared.CNY || updated.Initial.Minor() != 10000 {
		t.Fatalf("an edit changed the money: %+v", updated)
	}

	// A seeded account (BuiltinKey set) keeps its name: that name is a translation
	// key, so only the kind is editable on it.
	stored, err := f.store.Accounts().Get(f.bank)
	if err != nil {
		t.Fatal(err)
	}
	stored.BuiltinKey = domain.BuiltinAccountDebitCard
	if err := f.store.Accounts().Update(stored); err != nil {
		t.Fatal(err)
	}
	kept, err := f.svc.UpdateAccount(f.bank, "不许改名", domain.AccountWallet)
	if err != nil {
		t.Fatalf("editing a builtin account must be allowed: %v", err)
	}
	if kept.Name != "银行卡" {
		t.Fatalf("a builtin account was renamed to %q", kept.Name)
	}
	if kept.Kind != domain.AccountWallet {
		t.Fatalf("a builtin account's kind was not editable: %v", kept.Kind)
	}
	if err := f.svc.DeleteAccount(f.bank); shared.Code(err) != "ledger.account.builtin" {
		t.Fatalf("deleting a builtin account must be refused, got %v", err)
	}
	if _, err := f.svc.UpdateAccount(f.cash, "  ", domain.AccountCash); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a blank name must be refused, got %v", err)
	}
}

// BudgetRows is the budget tab's list: ceilings first, even before anything is spent,
// then the categories that spent money without one.
func TestBudgetRowsListCeilingsBeforeSpending(t *testing.T) {
	f := newFixture(t)
	month := shared.MonthOf(f.clock.Now())
	transport, err := f.svc.CreateCategory("交通", domain.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	// Dining has spending and a ceiling; transport has spending only; salary has a
	// ceiling and no spending at all (it is an income category, so it is skipped).
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(1000), AccountID: f.cash, CategoryID: f.dining,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateEntry(app.EntryInput{
		Kind: domain.EntryExpense, Amount: cny(5000), AccountID: f.cash, CategoryID: transport.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpsertBudget(f.dining, month, cny(20000)); err != nil {
		t.Fatal(err)
	}
	// A ceiling on a category nothing was spent in must still be listed.
	if _, err := f.svc.UpsertBudget(f.focusFine, month, cny(30000)); err != nil {
		t.Fatal(err)
	}

	rows, err := f.svc.BudgetRows(month)
	if err != nil {
		t.Fatalf("BudgetRows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want the two ceilings and the unbudgeted spender", rows)
	}
	// Ceilings first, most spent first, and a ceiling with no spending still appears.
	if rows[0].Budget == nil || rows[0].Category.ID != f.dining {
		t.Fatalf("the first row is not the ceiling that was spent against: %+v", rows[0])
	}
	if rows[0].Spent.Minor() != 1000 {
		t.Errorf("spent = %d on the first row, want 1000", rows[0].Spent.Minor())
	}
	if rows[1].Budget == nil || rows[1].Category.ID != f.focusFine {
		t.Fatalf("the unspent ceiling is not second: %+v", rows[1])
	}
	if rows[1].Spent.Minor() != 0 || rows[1].Budget.Limit.Minor() != 30000 {
		t.Errorf("an unspent ceiling reads %+v", rows[1])
	}
	if rows[1].Spent.Currency() != shared.CNY {
		t.Errorf("an unspent ceiling has no currency: %v", rows[1].Spent.Currency())
	}
	if rows[2].Budget != nil || rows[2].Category.ID != transport.ID {
		t.Fatalf("the unbudgeted spender is not last: %+v", rows[2])
	}
}
