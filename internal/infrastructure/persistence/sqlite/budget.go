package sqlite

import (
	ledger "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const budgetColumns = `id, category_id, month, limit_minor, currency`

// Upsert writes the budget for one category and month; the unique (category_id, month)
// constraint replaces the old row instead of adding a second one.
func (r BudgetRepo) Upsert(b *ledger.Budget) error {
	_, err := r.exec(`INSERT INTO budget (`+budgetColumns+`) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(category_id, month) DO UPDATE SET
			id = excluded.id,
			limit_minor = excluded.limit_minor,
			currency = excluded.currency`,
		string(b.ID), string(b.CategoryID), month(b.Month), b.Limit.Minor(),
		string(b.Limit.Currency()))
	return err
}

// Delete removes a budget.
func (r BudgetRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM budget WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.budget.not_found", "budget not found")
}

// ListByMonth returns the budgets of one month.
func (r BudgetRepo) ListByMonth(m shared.Month) ([]*ledger.Budget, error) {
	rows, err := r.query(`SELECT `+budgetColumns+` FROM budget WHERE month = ? ORDER BY id ASC`, month(m))
	if err != nil {
		return nil, err
	}
	return collectBudgets(rows)
}

// ListAll returns every budget, oldest month first.
func (r BudgetRepo) ListAll() ([]*ledger.Budget, error) {
	rows, err := r.query(`SELECT ` + budgetColumns + ` FROM budget ORDER BY month ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	return collectBudgets(rows)
}

// Get returns the budget of one category and month.
func (r BudgetRepo) Get(categoryID shared.ID, m shared.Month) (*ledger.Budget, error) {
	b, err := scanBudget(r.queryRow(`SELECT `+budgetColumns+` FROM budget
		WHERE category_id = ? AND month = ?`, string(categoryID), month(m)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("ledger.budget.not_found", "budget not found")
		}
		return nil, err
	}
	return b, nil
}

func collectBudgets(rows rowsScanner) ([]*ledger.Budget, error) {
	defer rows.Close()
	var out []*ledger.Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBudget(s scanner) (*ledger.Budget, error) {
	var (
		id, categoryID, monthText, currency string
		limitMinor                          int64
	)
	if err := s.Scan(&id, &categoryID, &monthText, &limitMinor, &currency); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan budget", err)
	}
	m, err := toMonth(monthText)
	if err != nil {
		return nil, err
	}
	return &ledger.Budget{
		ID:         shared.ID(id),
		CategoryID: shared.ID(categoryID),
		Month:      m,
		Limit:      shared.NewMoney(limitMinor, toCurrency(currency)),
	}, nil
}
