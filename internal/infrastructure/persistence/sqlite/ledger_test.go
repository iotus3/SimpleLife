package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

// seededLedger creates the accounts and categories the ledger tests share.
type seededLedger struct {
	cash, bank   shared.ID
	usd          shared.ID
	dining       shared.ID
	salary       shared.ID
	fineCategory shared.ID
	focusPot     shared.ID
}

func seedLedger(t *testing.T, db *sqlite.DB) *seededLedger {
	t.Helper()
	gen := &idGen{}
	cash, err := ledger.NewAccount(gen.New(), "现金", ledger.AccountCash, shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	bank, err := ledger.NewAccount(gen.New(), "银行卡", ledger.AccountDebitCard, shared.NewMoney(500000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	usd, err := ledger.NewAccount(gen.New(), "美元账户", ledger.AccountWallet, shared.NewMoney(5000, shared.USD))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []*ledger.Account{cash, bank, usd} {
		if err := db.Accounts().Create(a); err != nil {
			t.Fatalf("Create account: %v", err)
		}
	}
	dining, err := ledger.NewCategory(gen.New(), "餐饮", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	salary, err := ledger.NewCategory(gen.New(), "工资", ledger.CategoryIncome)
	if err != nil {
		t.Fatal(err)
	}
	fine, err := ledger.NewCategory(gen.New(), "罚金", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	pot := &ledger.Account{
		ID: gen.New(), Name: "专注储蓄", BuiltinKey: ledger.BuiltinAccountFocusPot,
		Kind: ledger.AccountSavings, Initial: shared.NewMoney(0, shared.CNY), SortOrder: 9,
	}
	if err := db.Accounts().Create(pot); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*ledger.Category{dining, salary, fine} {
		if err := db.Categories().Create(c); err != nil {
			t.Fatalf("Create category: %v", err)
		}
	}
	return &seededLedger{
		cash: cash.ID, bank: bank.ID, usd: usd.ID,
		dining: dining.ID, salary: salary.ID, fineCategory: fine.ID, focusPot: pot.ID,
	}
}

func TestAccountRoundTripAndOrdering(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)

	accounts, err := db.Accounts().List(false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(accounts) != 4 {
		t.Fatalf("accounts = %d", len(accounts))
	}
	// Sort order first: the focus pot has 9, the user accounts have 0.
	if accounts[len(accounts)-1].ID != seed.focusPot {
		t.Fatalf("the sort order must put the pot last: %+v", accounts[len(accounts)-1])
	}

	got, err := db.Accounts().Get(seed.cash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "现金" || got.Kind != ledger.AccountCash {
		t.Fatalf("account = %+v", got)
	}
	if got.Initial.Minor() != 10000 || got.Initial.Currency() != shared.CNY {
		t.Fatalf("money lost: %+v", got.Initial)
	}

	// Archiving hides the account from the default list but not from the full one.
	got.Archived = true
	if err := db.Accounts().Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	visible, err := db.Accounts().List(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range visible {
		if a.ID == seed.cash {
			t.Fatal("an archived account must be hidden")
		}
	}
	all, err := db.Accounts().List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("all = %d", len(all))
	}

	if err := db.Accounts().Delete(seed.bank); err != nil {
		t.Fatalf("Delete an unused account: %v", err)
	}
	if err := db.Accounts().Delete(seed.bank); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}
	if _, err := db.Accounts().Get("0000000000ACCT0000000099"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing account must be not-found, got %v", err)
	}
}

func TestAccountBuiltinKeyIsUnique(t *testing.T) {
	db := openFile(t)
	gen := &idGen{}
	first := &ledger.Account{ID: gen.New(), Name: "现金", BuiltinKey: ledger.BuiltinAccountCash,
		Kind: ledger.AccountCash, Initial: shared.NewMoney(0, shared.CNY)}
	if err := db.Accounts().Create(first); err != nil {
		t.Fatal(err)
	}
	second := &ledger.Account{ID: gen.New(), Name: "现金 2", BuiltinKey: ledger.BuiltinAccountCash,
		Kind: ledger.AccountCash, Initial: shared.NewMoney(0, shared.CNY)}
	if err := db.Accounts().Create(second); !errors.Is(err, shared.ErrDuplicate) {
		t.Fatalf("a duplicate builtin key must be refused, got %v", err)
	}
	// Two user accounts may share a name: only builtin keys are unique.
	third := &ledger.Account{ID: gen.New(), Name: "现金", Kind: ledger.AccountCash,
		Initial: shared.NewMoney(0, shared.CNY)}
	if err := db.Accounts().Create(third); err != nil {
		t.Fatalf("a user account with the same name must be allowed: %v", err)
	}
}

func TestCategoryTreeAndGuards(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}

	parent, err := ledger.NewCategory(gen.New(), "吃饭", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Categories().Create(parent); err != nil {
		t.Fatal(err)
	}
	child, err := ledger.NewCategory(gen.New(), "午饭", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.AttachParent(parent); err != nil {
		t.Fatal(err)
	}
	if err := db.Categories().Create(child); err != nil {
		t.Fatalf("Create child: %v", err)
	}
	got, err := db.Categories().Get(child.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Fatalf("parent lost: %+v", got)
	}
	if n, err := db.Categories().CountChildren(parent.ID); err != nil || n != 1 {
		t.Fatalf("children = %d, %v", n, err)
	}
	if n, err := db.Categories().CountEntries(parent.ID); err != nil || n != 0 {
		t.Fatalf("entries = %d, %v", n, err)
	}

	// A category with an entry cannot be deleted: the foreign key says so even if
	// the use case forgot to check.
	entry := sampleEntry(t, gen.New(), ledger.EntryInput{
		Kind: ledger.EntryExpense, Amount: shared.NewMoney(3550, shared.CNY),
		AccountID: seed.cash, CategoryID: child.ID,
	}, clock().Now())
	if err := db.Entries().Create(entry); err != nil {
		t.Fatal(err)
	}
	if n, err := db.Categories().CountEntries(child.ID); err != nil || n != 1 {
		t.Fatalf("entries = %d, %v", n, err)
	}
	if err := db.Categories().Delete(child.ID); err == nil {
		t.Fatal("an entry must block the category delete")
	}

	// Deleting the entry frees both.
	if err := db.Entries().Delete(entry.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Categories().Delete(child.ID); err != nil {
		t.Fatalf("Delete child: %v", err)
	}
	if err := db.Categories().Delete(parent.ID); err != nil {
		t.Fatalf("an empty parent must be deletable: %v", err)
	}
	if err := db.Categories().Delete(seed.dining); err != nil {
		t.Fatalf("an unused category must be deletable: %v", err)
	}

	list, err := db.Categories().List()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(list); i++ {
		if list[i].Direction < list[i-1].Direction {
			t.Fatal("categories must be grouped by direction")
		}
	}
	if _, err := db.Categories().Get("0000000000CATE0000000099"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing category must be not-found, got %v", err)
	}
}

func TestEntryFiltersAndAggregates(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}
	clk := clock()
	day := shared.DateOf(clk.Now())

	makeEntry := func(kind ledger.EntryKind, minor int64, currency shared.Currency, account, to, category shared.ID, at time.Time, note string) *ledger.Entry {
		t.Helper()
		e := sampleEntry(t, gen.New(), ledger.EntryInput{
			Kind: kind, Amount: shared.NewMoney(minor, currency), AccountID: account,
			ToAccountID: to, CategoryID: category, OccurredAt: at, Note: note, Tags: []string{"daily"},
		}, clk.Now())
		if err := db.Entries().Create(e); err != nil {
			t.Fatalf("Create entry: %v", err)
		}
		return e
	}

	makeEntry(ledger.EntryExpense, 3550, shared.CNY, seed.cash, "", seed.dining, day.At(12, 0), "午饭")
	makeEntry(ledger.EntryIncome, 500000, shared.CNY, seed.bank, "", seed.salary, day.At(9, 0), "工资")
	transfer := makeEntry(ledger.EntryTransfer, 10000, shared.CNY, seed.bank, seed.cash, "", day.At(10, 0), "取现")
	usdSpend := makeEntry(ledger.EntryExpense, 1200, shared.USD, seed.usd, "", seed.fineCategory, day.At(15, 0), "usd note")
	// One entry from the previous month, to prove the range filter works.
	old := makeEntry(ledger.EntryExpense, 999, shared.CNY, seed.cash, "", seed.dining, day.AddDays(-40).At(8, 0), "上个月")

	t.Run("list and order", func(t *testing.T) {
		got, err := db.Entries().List(ledger.EntryFilter{NewestFirst: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 5 {
			t.Fatalf("entries = %d", len(got))
		}
		if got[0].OccurredAt.Before(got[len(got)-1].OccurredAt) {
			t.Fatal("NewestFirst must put the newest entry first")
		}
		oldest, err := db.Entries().List(ledger.EntryFilter{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(oldest) != 1 || oldest[0].ID != old.ID {
			t.Fatalf("the oldest first = %+v", oldest)
		}
	})
	t.Run("range", func(t *testing.T) {
		from := day.Midnight()
		to := day.AddDays(1).Midnight()
		got, err := db.Entries().List(ledger.EntryFilter{From: from, To: to})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 {
			t.Fatalf("today = %d", len(got))
		}
		n, err := db.Entries().Count(ledger.EntryFilter{From: from, To: to})
		if err != nil || n != 4 {
			t.Fatalf("count = %d, %v", n, err)
		}
	})
	t.Run("account matches both sides", func(t *testing.T) {
		got, err := db.Entries().List(ledger.EntryFilter{AccountID: seed.cash})
		if err != nil {
			t.Fatal(err)
		}
		// dinner, the transfer's destination side, and the old entry.
		if len(got) != 3 {
			t.Fatalf("cash entries = %d", len(got))
		}
		var sawTransfer bool
		for _, e := range got {
			if e.ID == transfer.ID {
				sawTransfer = true
			}
		}
		if !sawTransfer {
			t.Fatal("a transfer must be visible from its destination account")
		}
	})
	t.Run("kind and currency", func(t *testing.T) {
		expenses, err := db.Entries().List(ledger.EntryFilter{Kind: []ledger.EntryKind{ledger.EntryExpense}})
		if err != nil {
			t.Fatal(err)
		}
		if len(expenses) != 3 {
			t.Fatalf("expenses = %d", len(expenses))
		}
		usd, err := db.Entries().List(ledger.EntryFilter{Currency: shared.USD})
		if err != nil {
			t.Fatal(err)
		}
		if len(usd) != 1 || usd[0].ID != usdSpend.ID {
			t.Fatalf("usd = %d", len(usd))
		}
	})
	t.Run("keyword", func(t *testing.T) {
		got, err := db.Entries().List(ledger.EntryFilter{Keyword: "NOTE"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != usdSpend.ID {
			t.Fatalf("keyword = %d", len(got))
		}
	})
	t.Run("aggregate by day keeps local days", func(t *testing.T) {
		got, err := db.Entries().Aggregate(ledger.EntryFilter{}, ledger.GroupDay)
		if err != nil {
			t.Fatal(err)
		}
		keys := map[string]int64{}
		for _, row := range got {
			keys[row.Key] += row.Sum.Minor()
		}
		// Dinner -3550, salary +500000, transfer -10000 (not counted as income or
		// expense by Summarise, but signed here), usd -1200.
		if keys[day.String()] != 500000-3550-10000-1200 {
			t.Fatalf("day total = %d (%v)", keys[day.String()], keys)
		}
		if keys[day.AddDays(-40).String()] != -999 {
			t.Fatalf("old day total = %d", keys[day.AddDays(-40).String()])
		}
	})
	t.Run("aggregate by category keeps currencies apart", func(t *testing.T) {
		got, err := db.Entries().Aggregate(ledger.EntryFilter{}, ledger.GroupCategory)
		if err != nil {
			t.Fatal(err)
		}
		type key struct {
			id  shared.ID
			cur shared.Currency
		}
		sums := map[key]int64{}
		for _, row := range got {
			sums[key{shared.ID(row.Key), row.Currency}] += row.Sum.Minor()
		}
		if sums[key{seed.dining, shared.CNY}] != -3550-999 {
			t.Fatalf("dining = %d", sums[key{seed.dining, shared.CNY}])
		}
		if sums[key{seed.fineCategory, shared.USD}] != -1200 {
			t.Fatalf("usd fine = %d", sums[key{seed.fineCategory, shared.USD}])
		}
	})
	t.Run("aggregate by account", func(t *testing.T) {
		got, err := db.Entries().Aggregate(ledger.EntryFilter{}, ledger.GroupAccount)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range got {
			if shared.ID(row.Key) == seed.bank {
				found = true
				// salary in, transfer out.
				if row.Sum.Minor() != 500000-10000 {
					t.Fatalf("bank = %d", row.Sum.Minor())
				}
			}
		}
		if !found {
			t.Fatal("the bank row is missing")
		}
	})
	t.Run("sum by origin", func(t *testing.T) {
		from, to := day.Midnight(), day.AddDays(1).Midnight()
		total, err := db.Entries().SumByOrigin(ledger.OriginManual, ledger.EntryIncome, from, to, shared.CNY)
		if err != nil {
			t.Fatal(err)
		}
		if total.Minor() != 500000 || total.Currency() != shared.CNY {
			t.Fatalf("total = %+v", total)
		}
		// A different currency must not be added in.
		empty, err := db.Entries().SumByOrigin(ledger.OriginManual, ledger.EntryIncome, from, to, shared.USD)
		if err != nil || empty.Minor() != 0 {
			t.Fatalf("usd total = %+v, %v", empty, err)
		}
		// And a window with nothing in it returns zero, not an error.
		none, err := db.Entries().SumByOrigin(ledger.OriginManual, ledger.EntryIncome, from.Add(-240*time.Hour), from.Add(-200*time.Hour), shared.CNY)
		if err != nil || none.Minor() != 0 {
			t.Fatalf("empty window = %+v, %v", none, err)
		}
	})
	t.Run("balances count both transfer sides", func(t *testing.T) {
		rows, err := db.Entries().AccountBalances(ledger.EntryFilter{})
		if err != nil {
			t.Fatal(err)
		}
		sums := map[shared.ID]int64{}
		for _, r := range rows {
			sums[r.AccountID] = r.Sum
		}
		// cash: -3550 (dinner) +10000 (transfer in) -999 (old)
		if sums[seed.cash] != -3550+10000-999 {
			t.Fatalf("cash = %d", sums[seed.cash])
		}
		// bank: +500000 -10000
		if sums[seed.bank] != 500000-10000 {
			t.Fatalf("bank = %d", sums[seed.bank])
		}
	})
}

func TestEntryFocusBookingIsUnique(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}
	sessionID := shared.ID("0000000000SESS0000000201")
	// The booking references its session, and the foreign key enforces that: the
	// focus service books money only after the session row is committed.
	session, err := focus.Start(sessionID, "", focus.KindFocus, 25*time.Minute, clock().Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Complete(clock().Now().Add(25 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Create(session); err != nil {
		t.Fatal(err)
	}

	first := sampleEntry(t, gen.New(), ledger.EntryInput{
		Kind: ledger.EntryIncome, Amount: shared.NewMoney(50, shared.CNY), AccountID: seed.focusPot,
		CategoryID: seed.salary, Origin: ledger.OriginFocusReward, FocusSessionID: sessionID,
	}, clock().Now())
	if err := db.Entries().Create(first); err != nil {
		t.Fatalf("Create: %v", err)
	}
	second := sampleEntry(t, gen.New(), ledger.EntryInput{
		Kind: ledger.EntryIncome, Amount: shared.NewMoney(50, shared.CNY), AccountID: seed.focusPot,
		CategoryID: seed.salary, Origin: ledger.OriginFocusReward, FocusSessionID: sessionID,
	}, clock().Now())
	if err := db.Entries().Create(second); !errors.Is(err, shared.ErrDuplicate) {
		t.Fatalf("a second booking for the same session must be refused, got %v", err)
	}
	if exists, err := db.Entries().ExistsFocusSession(sessionID); err != nil || !exists {
		t.Fatalf("ExistsFocusSession = %v, %v", exists, err)
	}
	if exists, err := db.Entries().ExistsFocusSession("0000000000SESS0000000299"); err != nil || exists {
		t.Fatalf("an unknown session = %v, %v", exists, err)
	}

	// Manual entries never collide, because the index only covers non-empty ids.
	for i := 0; i < 3; i++ {
		e := sampleEntry(t, gen.New(), ledger.EntryInput{
			Kind: ledger.EntryExpense, Amount: shared.NewMoney(100, shared.CNY),
			AccountID: seed.cash, CategoryID: seed.dining,
		}, clock().Now())
		if err := db.Entries().Create(e); err != nil {
			t.Fatalf("manual entry %d: %v", i, err)
		}
	}
}

func TestEntryConstraints(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}

	t.Run("missing account is refused by the foreign key", func(t *testing.T) {
		e := &ledger.Entry{
			ID: gen.New(), Kind: ledger.EntryExpense, Amount: shared.NewMoney(100, shared.CNY),
			AccountID: "0000000000ACCT0000000099", CategoryID: seed.dining, OccurredAt: clock().Now(),
			Origin: ledger.OriginManual, CreatedAt: clock().Now(),
		}
		if err := db.Entries().Create(e); err == nil {
			t.Fatal("an entry for an unknown account must be refused")
		}
	})
	t.Run("a zero amount is refused by the check constraint", func(t *testing.T) {
		e := &ledger.Entry{
			ID: gen.New(), Kind: ledger.EntryExpense, Amount: shared.NewMoney(0, shared.CNY),
			AccountID: seed.cash, CategoryID: seed.dining, OccurredAt: clock().Now(),
			Origin: ledger.OriginManual, CreatedAt: clock().Now(),
		}
		if err := db.Entries().Create(e); err == nil {
			t.Fatal("a zero amount must be refused")
		}
	})
	t.Run("deleting an account with entries is refused", func(t *testing.T) {
		e := sampleEntry(t, gen.New(), ledger.EntryInput{
			Kind: ledger.EntryExpense, Amount: shared.NewMoney(100, shared.CNY),
			AccountID: seed.cash, CategoryID: seed.dining,
		}, clock().Now())
		if err := db.Entries().Create(e); err != nil {
			t.Fatal(err)
		}
		if n, err := db.Accounts().CountEntries(seed.cash); err != nil || n != 1 {
			t.Fatalf("count = %d, %v", n, err)
		}
		if err := db.Accounts().Delete(seed.cash); err == nil {
			t.Fatal("the foreign key must refuse deleting an account with entries")
		}
	})
	t.Run("update and delete missing", func(t *testing.T) {
		e := sampleEntry(t, gen.New(), ledger.EntryInput{
			Kind: ledger.EntryExpense, Amount: shared.NewMoney(100, shared.CNY),
			AccountID: seed.cash, CategoryID: seed.dining,
		}, clock().Now())
		if err := db.Entries().Update(e); !shared.IsKind(err, shared.KindNotFound) {
			t.Fatalf("update missing = %v", err)
		}
		if err := db.Entries().Delete(e.ID); !shared.IsKind(err, shared.KindNotFound) {
			t.Fatalf("delete missing = %v", err)
		}
		if _, err := db.Entries().Get(e.ID); !shared.IsKind(err, shared.KindNotFound) {
			t.Fatalf("get missing = %v", err)
		}
	})
}

func TestEntryUpdateKeepsIdentity(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}
	e := sampleEntry(t, gen.New(), ledger.EntryInput{
		Kind: ledger.EntryExpense, Amount: shared.NewMoney(3550, shared.CNY),
		AccountID: seed.cash, CategoryID: seed.dining, Note: "午饭",
	}, clock().Now())
	if err := db.Entries().Create(e); err != nil {
		t.Fatal(err)
	}
	e.Note = "改成晚饭"
	updated, err := ledger.NewEntry(e.ID, clock().Now(), ledger.EntryInput{
		Kind: ledger.EntryIncome, Amount: shared.NewMoney(100, shared.CNY),
		AccountID: seed.bank, CategoryID: seed.salary, Note: "改成收入",
		OccurredAt: e.OccurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated.CreatedAt = e.CreatedAt
	if err := db.Entries().Update(updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := db.Entries().Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != ledger.EntryIncome || got.Amount.Minor() != 100 || got.AccountID != seed.bank {
		t.Fatalf("entry = %+v", got)
	}
	if got.Note != "改成收入" || len(got.Tags) != 0 {
		t.Fatalf("note/tags = %q %v", got.Note, got.Tags)
	}
}

func TestBudgetUpsertKeepsOneRowPerMonth(t *testing.T) {
	db := openFile(t)
	seed := seedLedger(t, db)
	gen := &idGen{}
	m := shared.MustParseMonth("2026-11")

	first, err := ledger.NewBudget(gen.New(), seed.dining, m, shared.NewMoney(100000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Budgets().Upsert(first); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// The second write replaces the first: two ceilings for one category and month
	// would make the report ambiguous.
	second, err := ledger.NewBudget(gen.New(), seed.dining, m, shared.NewMoney(120000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Budgets().Upsert(second); err != nil {
		t.Fatalf("Upsert again: %v", err)
	}
	list, err := db.Budgets().ListByMonth(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("budgets = %d", len(list))
	}
	if list[0].Limit.Minor() != 120000 || list[0].ID != second.ID {
		t.Fatalf("budget = %+v", list[0])
	}

	got, err := db.Budgets().Get(seed.dining, m)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Limit.Minor() != 120000 {
		t.Fatalf("limit = %d", got.Limit.Minor())
	}
	if _, err := db.Budgets().Get(seed.salary, m); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing budget must be not-found, got %v", err)
	}

	// Another month is a separate row.
	other, err := ledger.NewBudget(gen.New(), seed.dining, m.AddMonths(1), shared.NewMoney(90000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Budgets().Upsert(other); err != nil {
		t.Fatal(err)
	}
	all, err := db.Budgets().ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %d", len(all))
	}
	if all[0].Month != m {
		t.Fatalf("ListAll must be oldest month first: %+v", all)
	}

	if err := db.Budgets().Delete(second.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := db.Budgets().Delete(second.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}
}
