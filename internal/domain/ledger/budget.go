package ledger

import (
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Budget is a monthly spending ceiling for one category; the month/category pair is unique.
type Budget struct {
	ID         shared.ID
	CategoryID shared.ID
	Month      shared.Month
	Limit      shared.Money
}

// NewBudget builds a budget after validating its parts.
func NewBudget(id shared.ID, categoryID shared.ID, month shared.Month, limit shared.Money) (*Budget, error) {
	b := &Budget{ID: id, CategoryID: categoryID, Month: month, Limit: limit}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

// Validate checks the budget invariants.
func (b *Budget) Validate() error {
	if b.ID.IsZero() {
		return shared.Invalid("ledger.budget.id", "", "budget id is required")
	}
	if b.CategoryID.IsZero() {
		return shared.Invalid("ledger.budget.category", "category_id", "category is required")
	}
	if b.Month.IsZero() {
		return shared.Invalid("ledger.budget.month", "month", "month is required")
	}
	if !b.Limit.Currency().Valid() {
		return shared.Invalid("ledger.budget.currency", "limit", "currency must be CNY or USD")
	}
	if b.Limit.Minor() <= 0 {
		return shared.Invalid("ledger.budget.limit", "limit", "limit must be greater than zero")
	}
	return nil
}

// Usage is spent/limit as a fraction. A zero limit returns 0 rather than +Inf:
// the UI shows "no budget" instead of an infinite bar.
func (b *Budget) Usage(spent shared.Money) float64 {
	if b.Limit.Minor() <= 0 {
		return 0
	}
	if spent.Currency() != b.Limit.Currency() {
		return 0
	}
	// Both sides are integers, so this division is exact enough for a progress bar
	// and never produces NaN.
	return float64(spent.Abs().Minor()) / float64(b.Limit.Minor())
}

// Remaining is limit - spent, floored at zero (overspending is shown by Usage).
func (b *Budget) Remaining(spent shared.Money) shared.Money {
	if spent.Currency() != b.Limit.Currency() {
		return b.Limit
	}
	left := b.Limit.Minor() - spent.Abs().Minor()
	if left < 0 {
		left = 0
	}
	return shared.NewMoney(left, b.Limit.Currency())
}

// BudgetRepository is the persistence port for budgets.
type BudgetRepository interface {
	Upsert(b *Budget) error
	Delete(id shared.ID) error
	ListByMonth(month shared.Month) ([]*Budget, error)
	// ListAll returns every budget, for the export.
	ListAll() ([]*Budget, error)
	Get(categoryID shared.ID, month shared.Month) (*Budget, error)
}
