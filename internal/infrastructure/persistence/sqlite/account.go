package sqlite

import (
	"database/sql"

	ledger "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const accountColumns = `id, name, builtin_key, kind, initial_minor, currency, archived, sort_order`

// Create inserts an account. The partial unique index on builtin_key keeps the
// seed from running twice.
func (r AccountRepo) Create(a *ledger.Account) error {
	_, err := r.exec(`INSERT INTO account (`+accountColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(a.ID), a.Name, a.BuiltinKey, int(a.Kind), a.Initial.Minor(),
		string(a.Initial.Currency()), boolToInt(a.Archived), a.SortOrder)
	return err
}

// Update rewrites an account.
func (r AccountRepo) Update(a *ledger.Account) error {
	res, err := r.exec(`UPDATE account SET name = ?, builtin_key = ?, kind = ?, initial_minor = ?,
		currency = ?, archived = ?, sort_order = ? WHERE id = ?`,
		a.Name, a.BuiltinKey, int(a.Kind), a.Initial.Minor(), string(a.Initial.Currency()),
		boolToInt(a.Archived), a.SortOrder, string(a.ID))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.account.not_found", "account not found")
}

// Delete removes an account that has no entries. The foreign key refuses the delete
// even if the use case's guard was bypassed.
func (r AccountRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM account WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.account.not_found", "account not found")
}

// List returns accounts in display order: sort order, then name. Sorting here
// rather than in the UI keeps the export and the page in the same order.
func (r AccountRepo) List(includeArchived bool) ([]*ledger.Account, error) {
	query := `SELECT ` + accountColumns + ` FROM account`
	if !includeArchived {
		query += ` WHERE archived = 0`
	}
	query += ` ORDER BY sort_order ASC, name ASC, id ASC`
	rows, err := r.query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ledger.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get returns one account.
func (r AccountRepo) Get(id shared.ID) (*ledger.Account, error) {
	a, err := scanAccount(r.queryRow(`SELECT `+accountColumns+` FROM account WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("ledger.account.not_found", "account not found")
		}
		return nil, err
	}
	return a, nil
}

// CountEntries counts entries touching the account, both sides of a transfer.
func (r AccountRepo) CountEntries(id shared.ID) (int, error) {
	var n int
	err := r.queryRow(`SELECT COUNT(*) FROM entry WHERE account_id = ? OR to_account_id = ?`,
		string(id), string(id)).Scan(&n)
	if err != nil {
		return 0, mapError("count account entries", err)
	}
	return n, nil
}

func scanAccount(s scanner) (*ledger.Account, error) {
	var (
		id, name, builtin, currency string
		kind, archived              int
		initialMinor                int64
		sortOrder                   int
	)
	if err := s.Scan(&id, &name, &builtin, &kind, &initialMinor, &currency, &archived, &sortOrder); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan account", err)
	}
	return &ledger.Account{
		ID:         shared.ID(id),
		Name:       name,
		BuiltinKey: builtin,
		Kind:       ledger.AccountKind(kind),
		Initial:    shared.NewMoney(initialMinor, toCurrency(currency)),
		Archived:   intToBool(archived),
		SortOrder:  sortOrder,
	}, nil
}

// ── categories ─────────────────────────────────────────────────────────

const categoryColumns = `id, name, builtin_key, direction, parent_id, sort_order`

// Create inserts a category.
func (r CategoryRepo) Create(c *ledger.Category) error {
	_, err := r.exec(`INSERT INTO category (`+categoryColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		string(c.ID), c.Name, c.BuiltinKey, int(c.Direction), nullable(c.ParentID), c.SortOrder)
	return err
}

// Update rewrites a category.
func (r CategoryRepo) Update(c *ledger.Category) error {
	res, err := r.exec(`UPDATE category SET name = ?, builtin_key = ?, direction = ?,
		parent_id = ?, sort_order = ? WHERE id = ?`,
		c.Name, c.BuiltinKey, int(c.Direction), nullable(c.ParentID), c.SortOrder, string(c.ID))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.category.not_found", "category not found")
}

// Delete removes an unused category. ON DELETE RESTRICT protects the entries the
// use case already checked for.
func (r CategoryRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM category WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.category.not_found", "category not found")
}

// List returns categories grouped by direction, then by sort order and name.
func (r CategoryRepo) List() ([]*ledger.Category, error) {
	rows, err := r.query(`SELECT ` + categoryColumns + ` FROM category
		ORDER BY direction ASC, sort_order ASC, name ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ledger.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one category.
func (r CategoryRepo) Get(id shared.ID) (*ledger.Category, error) {
	c, err := scanCategory(r.queryRow(`SELECT `+categoryColumns+` FROM category WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("ledger.category.not_found", "category not found")
		}
		return nil, err
	}
	return c, nil
}

// CountEntries counts entries referencing the category.
func (r CategoryRepo) CountEntries(id shared.ID) (int, error) {
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM entry WHERE category_id = ?`, string(id)).Scan(&n); err != nil {
		return 0, mapError("count category entries", err)
	}
	return n, nil
}

// CountChildren counts sub-categories.
func (r CategoryRepo) CountChildren(id shared.ID) (int, error) {
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM category WHERE parent_id = ?`, string(id)).Scan(&n); err != nil {
		return 0, mapError("count category children", err)
	}
	return n, nil
}

func scanCategory(s scanner) (*ledger.Category, error) {
	var (
		id, name, builtin string
		direction, order  int
		parent            sql.NullString
	)
	if err := s.Scan(&id, &name, &builtin, &direction, &parent, &order); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan category", err)
	}
	return &ledger.Category{
		ID:         shared.ID(id),
		Name:       name,
		BuiltinKey: builtin,
		Direction:  ledger.CategoryDirection(direction),
		ParentID:   idOrEmpty(parent),
		SortOrder:  order,
	}, nil
}
