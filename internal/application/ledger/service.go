// Package ledger implements the money use cases. Every aggregate returns one subtotal
// per currency: CNY and USD are never converted.
package ledger

import (
	"sort"
	"time"

	"github.com/iotus3/simplelife/app/internal/contracts"
	domain "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Service is the ledger application service.
type Service struct {
	entries    domain.EntryRepository
	accounts   domain.AccountRepository
	categories domain.CategoryRepository
	budgets    domain.BudgetRepository

	tx    shared.TxRunner
	clock shared.Clock
	ids   shared.IDGen
}

// New wires the service.
func New(
	entries domain.EntryRepository,
	accounts domain.AccountRepository,
	categories domain.CategoryRepository,
	budgets domain.BudgetRepository,
	tx shared.TxRunner,
	clock shared.Clock,
	ids shared.IDGen,
) *Service {
	return &Service{entries: entries, accounts: accounts, categories: categories, budgets: budgets, tx: tx, clock: clock, ids: ids}
}

// EntryInput is the form data for one ledger line.
type EntryInput struct {
	Kind        domain.EntryKind
	Amount      shared.Money
	AccountID   shared.ID
	ToAccountID shared.ID
	CategoryID  shared.ID
	OccurredAt  time.Time
	Note        string
	Tags        []string
}

// CreateEntry validates the line, checks the category direction against the entry
// kind, and stores it.
func (s *Service) CreateEntry(in EntryInput) (*domain.Entry, error) {
	now := s.clock.Now()
	entry, err := s.buildEntry(in, now)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithinTx(func() error { return s.entries.Create(entry) }); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *Service) buildEntry(in EntryInput, now time.Time) (*domain.Entry, error) {
	// The category direction is checked here because only the service can load the
	// category; the aggregate checks everything that is local to the entry.
	if in.Kind == domain.EntryExpense || in.Kind == domain.EntryIncome {
		category, err := s.categories.Get(in.CategoryID)
		if err != nil {
			return nil, err
		}
		want := domain.CategoryExpense
		if in.Kind == domain.EntryIncome {
			want = domain.CategoryIncome
		}
		if category.Direction != want {
			return nil, shared.Invalid("ledger.entry.category_direction", "category_id", "category direction does not match the entry kind")
		}
	}
	if !in.AccountID.IsZero() {
		if _, err := s.accounts.Get(in.AccountID); err != nil {
			return nil, err
		}
		currency, err := s.accountCurrency(in.AccountID)
		if err != nil {
			return nil, err
		}
		if currency != in.Amount.Currency() {
			return nil, shared.Invalid("ledger.entry.currency_mismatch", "amount", "the amount must use the account currency")
		}
	}
	return domain.NewEntry(s.ids.New(), now, domain.EntryInput{
		Kind:        in.Kind,
		Amount:      in.Amount,
		AccountID:   in.AccountID,
		ToAccountID: in.ToAccountID,
		CategoryID:  in.CategoryID,
		OccurredAt:  in.OccurredAt,
		Note:        in.Note,
		Tags:        in.Tags,
	})
}

func (s *Service) accountCurrency(id shared.ID) (shared.Currency, error) {
	a, err := s.accounts.Get(id)
	if err != nil {
		return "", err
	}
	return a.Currency(), nil
}

// UpdateEntry replaces an entry's editable fields. The id and creation time are
// preserved; everything else goes through the same validation as a new entry.
func (s *Service) UpdateEntry(id shared.ID, in EntryInput) (*domain.Entry, error) {
	now := s.clock.Now()
	var out *domain.Entry
	err := s.tx.WithinTx(func() error {
		current, err := s.entries.Get(id)
		if err != nil {
			return err
		}
		sameDay := shared.DateOf(current.OccurredAt.Local()).Equal(shared.DateOf(now))
		if !sameDay && current.Origin == domain.OriginFocusReward {
			return shared.Conflict("ledger.entry.locked", "a booking from a focus session cannot be moved to another day")
		}
		rebuilt, err := s.buildEntry(in, now)
		if err != nil {
			return err
		}
		rebuilt.ID = current.ID
		rebuilt.CreatedAt = current.CreatedAt
		rebuilt.Origin = current.Origin
		rebuilt.FocusSessionID = current.FocusSessionID
		if err := s.entries.Update(rebuilt); err != nil {
			return err
		}
		out = rebuilt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteEntry removes a line. Bookings made by a focus session are protected: the
// user can edit them, but deleting one would silently undo a pomodoro and confuse
// the daily cap.
func (s *Service) DeleteEntry(id shared.ID) error {
	return s.tx.WithinTx(func() error {
		current, err := s.entries.Get(id)
		if err != nil {
			return err
		}
		if current.Origin == domain.OriginFocusReward {
			return shared.Conflict("ledger.entry.locked", "a booking from a focus session cannot be deleted")
		}
		return s.entries.Delete(id)
	})
}

// List returns entries matching the filter.
func (s *Service) List(f domain.EntryFilter) ([]*domain.Entry, error) { return s.entries.List(f) }

// MonthFilter builds the filter for one month.
func MonthFilter(m shared.Month) domain.EntryFilter {
	return domain.EntryFilter{From: m.Start(), To: m.End().Add(time.Millisecond), NewestFirst: true}
}

// Summary is one month's totals, per currency.
type Summary struct {
	Month      shared.Month
	Income     []shared.MoneyTotal
	Expense    []shared.MoneyTotal
	Net        []shared.MoneyTotal
	EntryCount int
}

// Summarise aggregates one month. Transfers are excluded from income and expense
// by construction: moving money between your own accounts is not earning or
// spending it, and counting it would double the month's numbers.
func (s *Service) Summarise(m shared.Month) (Summary, error) {
	out := Summary{Month: m}
	entries, err := s.entries.List(MonthFilter(m))
	if err != nil {
		return out, err
	}
	var income, expense shared.Tally
	for _, e := range entries {
		out.EntryCount++
		switch e.Kind {
		case domain.EntryIncome:
			income.Add(e.Amount)
		case domain.EntryExpense:
			expense.Add(e.Amount)
		}
	}
	out.Income = income.Totals()
	out.Expense = expense.Totals()
	out.Net = netTotals(out.Income, out.Expense)
	return out, nil
}

// netTotals subtracts expense from income per currency, keeping the order of the
// income list and appending currencies that only appear on the expense side.
func netTotals(income, expense []shared.MoneyTotal) []shared.MoneyTotal {
	byCurrency := make(map[shared.Currency]int64, len(income)+len(expense))
	var order []shared.Currency
	for _, t := range income {
		if _, ok := byCurrency[t.Currency]; !ok {
			order = append(order, t.Currency)
		}
		byCurrency[t.Currency] += t.Amount.Minor()
	}
	for _, t := range expense {
		if _, ok := byCurrency[t.Currency]; !ok {
			order = append(order, t.Currency)
		}
		byCurrency[t.Currency] -= t.Amount.Minor()
	}
	out := make([]shared.MoneyTotal, 0, len(order))
	for _, c := range order {
		out = append(out, shared.MoneyTotal{Currency: c, Amount: shared.NewMoney(byCurrency[c], c)})
	}
	return out
}

// AccountBalance is one account with its computed balance.
type AccountBalance struct {
	Account domain.Account
	Balance shared.Money
}

// Balances returns every account with its balance: opening balance plus the net
// movement of all entries, transfer credits included.
func (s *Service) Balances(includeArchived bool) ([]AccountBalance, error) {
	accounts, err := s.accounts.List(includeArchived)
	if err != nil {
		return nil, err
	}
	rows, err := s.entries.AccountBalances(domain.EntryFilter{})
	if err != nil {
		return nil, err
	}
	deltas := make(map[shared.ID]int64, len(rows))
	for _, r := range rows {
		deltas[r.AccountID] += r.Sum
	}
	out := make([]AccountBalance, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, AccountBalance{
			Account: *a,
			Balance: shared.NewMoney(a.Initial.Minor()+deltas[a.ID], a.Currency()),
		})
	}
	return out, nil
}

// CategorySpend is one category's spending in a month.
type CategorySpend struct {
	Category domain.Category
	Spent    shared.Money
	Budget   *domain.Budget
}

// CategorySpending returns expenses grouped by category for one month, with the
// budget attached when one exists. Ordering is by amount spent, largest first, so
// the page shows where the money actually went.
func (s *Service) CategorySpending(m shared.Month) ([]CategorySpend, error) {
	categories, err := s.categories.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[shared.ID]domain.Category, len(categories))
	for _, c := range categories {
		byID[c.ID] = *c
	}
	rows, err := s.entries.Aggregate(MonthFilter(m), domain.GroupCategory)
	if err != nil {
		return nil, err
	}
	budgets, err := s.budgets.ListByMonth(m)
	if err != nil {
		return nil, err
	}
	budgetByCategory := make(map[shared.ID]*domain.Budget, len(budgets))
	for _, b := range budgets {
		budgetByCategory[b.CategoryID] = b
	}
	out := make([]CategorySpend, 0, len(rows))
	for _, r := range rows {
		id := shared.ID(r.Key)
		category, ok := byID[id]
		if !ok {
			continue // an entry whose category was deleted; skip rather than crash
		}
		if r.Sum.Minor() >= 0 {
			continue // only expenses are "spending"
		}
		out = append(out, CategorySpend{
			Category: category,
			Spent:    shared.NewMoney(-r.Sum.Minor(), r.Sum.Currency()),
			Budget:   budgetByCategory[id],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Spent.Minor() > out[j].Spent.Minor() })
	return out, nil
}

// Book writes the money a pomodoro earned or cost; the amount is decided elsewhere.
func (s *Service) Book(req contracts.BookingRequest) error {
	now := s.clock.Now()
	kind := domain.EntryIncome
	if req.Direction == contracts.BookingExpense {
		kind = domain.EntryExpense
	}
	entry, err := domain.NewEntry(s.ids.New(), now, domain.EntryInput{
		Kind:           kind,
		Amount:         req.Amount,
		AccountID:      req.AccountID,
		CategoryID:     req.CategoryID,
		OccurredAt:     req.OccurredAt,
		Note:           req.Note,
		Origin:         domain.OriginFocusReward,
		FocusSessionID: req.SessionID,
	})
	if err != nil {
		return err
	}
	return s.entries.Create(entry)
}

// ── accounts ───────────────────────────────────────────────────────────

// CreateAccount adds an account.
func (s *Service) CreateAccount(name string, kind domain.AccountKind, initial shared.Money) (*domain.Account, error) {
	a, err := domain.NewAccount(s.ids.New(), name, kind, initial)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithinTx(func() error { return s.accounts.Create(a) }); err != nil {
		return nil, err
	}
	return a, nil
}

// RenameAccount renames a user account.
func (s *Service) RenameAccount(id shared.ID, name string) (*domain.Account, error) {
	var out *domain.Account
	err := s.tx.WithinTx(func() error {
		a, err := s.accounts.Get(id)
		if err != nil {
			return err
		}
		if err := a.Rename(name); err != nil {
			return err
		}
		if err := s.accounts.Update(a); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}

// UpdateAccount changes the name and the kind. Currency and opening balance are fixed at
// creation, and a seeded account keeps its name because that name is a translation key.
func (s *Service) UpdateAccount(id shared.ID, name string, kind domain.AccountKind) (*domain.Account, error) {
	var out *domain.Account
	err := s.tx.WithinTx(func() error {
		a, err := s.accounts.Get(id)
		if err != nil {
			return err
		}
		if a.BuiltinKey == "" {
			a.Name = name
		}
		a.Kind = kind
		if err := a.Validate(); err != nil {
			return err
		}
		if err := s.accounts.Update(a); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}

// ArchiveAccount hides an account that is no longer used.
func (s *Service) ArchiveAccount(id shared.ID) error {
	return s.tx.WithinTx(func() error {
		a, err := s.accounts.Get(id)
		if err != nil {
			return err
		}
		if err := a.Archive(); err != nil {
			return err
		}
		return s.accounts.Update(a)
	})
}

// DeleteAccount removes an account with no entries. A seeded account, and one that
// entries still point at, are refused, and the page says why.
func (s *Service) DeleteAccount(id shared.ID) error {
	return s.tx.WithinTx(func() error {
		account, err := s.accounts.Get(id)
		if err != nil {
			return err
		}
		if account.BuiltinKey != "" {
			return shared.Conflict("ledger.account.builtin", "a builtin account cannot be deleted")
		}
		count, err := s.accounts.CountEntries(id)
		if err != nil {
			return err
		}
		if count > 0 {
			return shared.Conflict("ledger.account.in_use", "the account still has entries")
		}
		return s.accounts.Delete(id)
	})
}

// Accounts lists accounts.
func (s *Service) Accounts(includeArchived bool) ([]*domain.Account, error) {
	return s.accounts.List(includeArchived)
}

// Account loads one account.
func (s *Service) Account(id shared.ID) (*domain.Account, error) { return s.accounts.Get(id) }

// ── categories ─────────────────────────────────────────────────────────

// CreateCategory adds a category, optionally nested one level.
func (s *Service) CreateCategory(name string, dir domain.CategoryDirection, parentID shared.ID) (*domain.Category, error) {
	c, err := domain.NewCategory(s.ids.New(), name, dir)
	if err != nil {
		return nil, err
	}
	if !parentID.IsZero() {
		parent, err := s.categories.Get(parentID)
		if err != nil {
			return nil, err
		}
		if err := c.AttachParent(parent); err != nil {
			return nil, err
		}
	}
	if err := s.tx.WithinTx(func() error { return s.categories.Create(c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// RenameCategory renames a user category.
func (s *Service) RenameCategory(id shared.ID, name string) (*domain.Category, error) {
	var out *domain.Category
	err := s.tx.WithinTx(func() error {
		c, err := s.categories.Get(id)
		if err != nil {
			return err
		}
		if err := c.Rename(name); err != nil {
			return err
		}
		if err := s.categories.Update(c); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, err
}

// DeleteCategory removes an unused category. The delete guard is the whole point:
// silently dropping a category would orphan historical entries.
func (s *Service) DeleteCategory(id shared.ID) error {
	return s.tx.WithinTx(func() error {
		current, err := s.categories.Get(id)
		if err != nil {
			return err
		}
		if current.BuiltinKey != "" {
			return shared.Conflict("ledger.category.builtin", "a builtin category cannot be deleted")
		}
		entries, err := s.categories.CountEntries(id)
		if err != nil {
			return err
		}
		if entries > 0 {
			return shared.Conflict("ledger.category.in_use", "the category still has entries")
		}
		children, err := s.categories.CountChildren(id)
		if err != nil {
			return err
		}
		if children > 0 {
			return shared.Conflict("ledger.category.has_children", "the category still has sub-categories")
		}
		return s.categories.Delete(id)
	})
}

// Categories lists every category.
func (s *Service) Categories() ([]*domain.Category, error) { return s.categories.List() }

// Category loads one category.
func (s *Service) Category(id shared.ID) (*domain.Category, error) { return s.categories.Get(id) }

// ── budgets ────────────────────────────────────────────────────────────

// UpsertBudget sets the ceiling for one category in one month.
func (s *Service) UpsertBudget(categoryID shared.ID, m shared.Month, limit shared.Money) (*domain.Budget, error) {
	existing, err := s.budgets.Get(categoryID, m)
	if err != nil && !shared.IsKind(err, shared.KindNotFound) {
		return nil, err
	}
	b := existing
	if b == nil {
		b = &domain.Budget{ID: s.ids.New(), CategoryID: categoryID, Month: m}
	}
	b.Limit = limit
	if err := b.Validate(); err != nil {
		return nil, err
	}
	if err := s.tx.WithinTx(func() error { return s.budgets.Upsert(b) }); err != nil {
		return nil, err
	}
	return b, nil
}

// DeleteBudget removes a ceiling.
func (s *Service) DeleteBudget(id shared.ID) error {
	return s.tx.WithinTx(func() error { return s.budgets.Delete(id) })
}

// Budgets lists the ceilings of one month.
func (s *Service) Budgets(m shared.Month) ([]*domain.Budget, error) { return s.budgets.ListByMonth(m) }

// BudgetRow is one line of the budget tab: a category, what it has cost this month,
// and the ceiling set for it (nil when there is none).
type BudgetRow struct {
	Category domain.Category
	Spent    shared.Money
	Budget   *domain.Budget
}

// BudgetRows lists the month's ceilings (most spent first), then the categories that spent
// without one, ties broken by name. The list is built from the budgets, not from the
// spending, so a ceiling appears the moment it is set.
func (s *Service) BudgetRows(m shared.Month) ([]BudgetRow, error) {
	spending, err := s.CategorySpending(m)
	if err != nil {
		return nil, err
	}
	budgets, err := s.budgets.ListByMonth(m)
	if err != nil {
		return nil, err
	}
	categories, err := s.categories.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[shared.ID]domain.Category, len(categories))
	for _, c := range categories {
		byID[c.ID] = *c
	}
	spentByCategory := make(map[shared.ID]shared.Money, len(spending))
	unbudgeted := make(map[shared.ID]bool, len(spending))
	for _, row := range spending {
		spentByCategory[row.Category.ID] = row.Spent
		unbudgeted[row.Category.ID] = true
	}
	rows := make([]BudgetRow, 0, len(budgets)+len(spending))
	for _, b := range budgets {
		category, ok := byID[b.CategoryID]
		if !ok {
			continue // a budget whose category is gone; the FK cascade removes it
		}
		spent, ok := spentByCategory[b.CategoryID]
		if !ok {
			spent = shared.ZeroMoney(b.Limit.Currency())
		}
		delete(unbudgeted, b.CategoryID)
		rows = append(rows, BudgetRow{Category: category, Spent: spent, Budget: b})
	}
	for id := range unbudgeted {
		category, ok := byID[id]
		if !ok || category.Direction != domain.CategoryExpense {
			continue
		}
		rows = append(rows, BudgetRow{Category: category, Spent: spentByCategory[id]})
	}
	budgeted := func(row BudgetRow) int {
		if row.Budget != nil {
			return 0
		}
		return 1
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if budgeted(rows[i]) != budgeted(rows[j]) {
			return budgeted(rows[i]) < budgeted(rows[j])
		}
		if rows[i].Spent.Minor() != rows[j].Spent.Minor() {
			return rows[i].Spent.Minor() > rows[j].Spent.Minor()
		}
		return rows[i].Category.Name < rows[j].Category.Name
	})
	return rows, nil
}

// Recent returns the newest entries, for the ledger page.
func (s *Service) Recent(limit int) ([]*domain.Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.entries.List(domain.EntryFilter{Limit: limit, NewestFirst: true})
}

// Search returns entries matching a keyword.
func (s *Service) Search(keyword string, limit int) ([]*domain.Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.entries.List(domain.EntryFilter{Keyword: keyword, Limit: limit, NewestFirst: true})
}
