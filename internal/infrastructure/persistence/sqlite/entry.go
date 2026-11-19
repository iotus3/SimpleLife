package sqlite

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	ledger "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const entryColumns = `id, kind, amount_minor, currency, account_id, to_account_id, category_id,
	occurred_at, note, tags, origin, focus_session_id, created_at`

// Create inserts a ledger line. The partial unique index on focus_session_id is
// what makes "one pomodoro books money once" true even under a double click.
func (r EntryRepo) Create(e *ledger.Entry) error {
	_, err := r.exec(`INSERT INTO entry (`+entryColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(e.ID), int(e.Kind), e.Amount.Minor(), string(e.Amount.Currency()),
		string(e.AccountID), nullable(e.ToAccountID), nullable(e.CategoryID),
		ms(e.OccurredAt), e.Note, tags(e.Tags), int(e.Origin),
		nullable(e.FocusSessionID), ms(e.CreatedAt))
	return err
}

// Update rewrites a line.
func (r EntryRepo) Update(e *ledger.Entry) error {
	res, err := r.exec(`UPDATE entry SET kind = ?, amount_minor = ?, currency = ?,
		account_id = ?, to_account_id = ?, category_id = ?, occurred_at = ?, note = ?,
		tags = ?, origin = ?, focus_session_id = ? WHERE id = ?`,
		int(e.Kind), e.Amount.Minor(), string(e.Amount.Currency()), string(e.AccountID),
		nullable(e.ToAccountID), nullable(e.CategoryID), ms(e.OccurredAt), e.Note,
		tags(e.Tags), int(e.Origin), nullable(e.FocusSessionID), string(e.ID))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.entry.not_found", "entry not found")
}

// Delete removes a line.
func (r EntryRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM entry WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "ledger.entry.not_found", "entry not found")
}

// Get returns one line.
func (r EntryRepo) Get(id shared.ID) (*ledger.Entry, error) {
	e, err := scanEntry(r.queryRow(`SELECT `+entryColumns+` FROM entry WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("ledger.entry.not_found", "entry not found")
		}
		return nil, err
	}
	return e, nil
}

// List returns the matching lines in the requested order.
func (r EntryRepo) List(f ledger.EntryFilter) ([]*ledger.Entry, error) {
	where, args := entryWhere(f)
	order := " ORDER BY occurred_at ASC, id ASC"
	if f.NewestFirst {
		order = " ORDER BY occurred_at DESC, id DESC"
	}
	rows, err := r.query(`SELECT `+entryColumns+` FROM entry`+where+order+limitClause(f.Limit, f.Offset), args...)
	if err != nil {
		return nil, err
	}
	return collectEntries(rows)
}

// Count counts the matching lines.
func (r EntryRepo) Count(f ledger.EntryFilter) (int, error) {
	where, args := entryWhere(f)
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM entry`+where, args...).Scan(&n); err != nil {
		return 0, mapError("count entries", err)
	}
	return n, nil
}

// Aggregate groups the matching lines: the range scan is SQL, the grouping is Go, because
// SQLite has no idea which time zone the app runs in.
func (r EntryRepo) Aggregate(f ledger.EntryFilter, groupBy ledger.AggregateGroup) ([]ledger.AggregateRow, error) {
	where, args := entryWhere(f)
	rows, err := r.query(`SELECT `+entryColumns+` FROM entry`+where, args...)
	if err != nil {
		return nil, err
	}
	entries, err := collectEntries(rows)
	if err != nil {
		return nil, err
	}
	type bucket struct {
		sum   int64
		count int
	}
	sums := make(map[string]*bucket)
	order := make([]string, 0, len(entries))
	currency := make(map[string]shared.Currency, len(entries))
	for _, e := range entries {
		key := aggregateKey(e, groupBy)
		// The currency is part of the bucket key: a CNY total and a USD total can
		// never be added together (ADR-011).
		full := key + "\x00" + string(e.Amount.Currency())
		b, ok := sums[full]
		if !ok {
			b = &bucket{}
			sums[full] = b
			order = append(order, full)
			currency[full] = e.Amount.Currency()
		}
		b.sum += e.SignedAmount()
		b.count++
	}
	sort.Strings(order)
	out := make([]ledger.AggregateRow, 0, len(order))
	for _, full := range order {
		key := full
		if i := strings.IndexByte(full, 0); i >= 0 {
			key = full[:i]
		}
		out = append(out, ledger.AggregateRow{
			Key:      key,
			Sum:      shared.NewMoney(sums[full].sum, currency[full]),
			Count:    sums[full].count,
			Currency: currency[full],
		})
	}
	return out, nil
}

func aggregateKey(e *ledger.Entry, groupBy ledger.AggregateGroup) string {
	switch groupBy {
	case ledger.GroupDay:
		return shared.DateOf(e.OccurredAt.Local()).String()
	case ledger.GroupMonth:
		return shared.MonthOf(e.OccurredAt.Local()).String()
	case ledger.GroupCategory:
		return string(e.CategoryID)
	case ledger.GroupAccount:
		return string(e.AccountID)
	}
	return ""
}

// SumByOrigin totals one origin and direction in a period, in one currency. The sum happens in
// SQL: the daily cap is checked on every completion, and the ledger only grows.
func (r EntryRepo) SumByOrigin(origin ledger.Origin, kind ledger.EntryKind, from, to time.Time, currency shared.Currency) (shared.Money, error) {
	var total int64
	err := r.queryRow(`SELECT COALESCE(SUM(amount_minor), 0) FROM entry
		WHERE origin = ? AND kind = ? AND currency = ? AND occurred_at >= ? AND occurred_at < ?`,
		int(origin), int(kind), string(currency), ms(from), ms(to)).Scan(&total)
	if err != nil {
		return shared.NewMoney(0, currency), mapError("sum entries by origin", err)
	}
	return shared.NewMoney(total, currency), nil
}

// ExistsFocusSession reports whether a session already produced a line.
func (r EntryRepo) ExistsFocusSession(sessionID shared.ID) (bool, error) {
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM entry WHERE focus_session_id = ?`,
		string(sessionID)).Scan(&n); err != nil {
		return false, mapError("count session entries", err)
	}
	return n > 0, nil
}

// AccountBalances returns each account's net movement, transfer credits included, which is why
// it cannot be a plain GROUP BY over one column.
func (r EntryRepo) AccountBalances(f ledger.EntryFilter) ([]ledger.AccountBalanceRow, error) {
	where, args := entryWhere(f)
	rows, err := r.query(`SELECT `+entryColumns+` FROM entry`+where, args...)
	if err != nil {
		return nil, err
	}
	entries, err := collectEntries(rows)
	if err != nil {
		return nil, err
	}
	type bucket struct {
		sum      int64
		currency shared.Currency
	}
	sums := make(map[shared.ID]*bucket, 8)
	add := func(id shared.ID, delta int64, currency shared.Currency) {
		if id.IsZero() {
			return
		}
		b, ok := sums[id]
		if !ok {
			b = &bucket{currency: currency}
			sums[id] = b
		}
		b.sum += delta
	}
	for _, e := range entries {
		currency := e.Amount.Currency()
		add(e.AccountID, e.SignedAmount(), currency)
		if e.Kind == ledger.EntryTransfer {
			add(e.ToAccountID, e.Amount.Minor(), currency)
		}
	}
	ids := make([]shared.ID, 0, len(sums))
	for id := range sums {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]ledger.AccountBalanceRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, ledger.AccountBalanceRow{
			AccountID: id,
			Sum:       sums[id].sum,
			Currency:  sums[id].currency,
		})
	}
	return out, nil
}

func collectEntries(rows *sql.Rows) ([]*ledger.Entry, error) {
	defer rows.Close()
	var out []*ledger.Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEntry(s scanner) (*ledger.Entry, error) {
	var (
		id, currency, accountID, note string
		kind, origin                  int
		amountMinor, occurredAt       int64
		createdAt                     int64
		toAccount, category, session  sql.NullString
		tagJSON                       string
	)
	if err := s.Scan(&id, &kind, &amountMinor, &currency, &accountID, &toAccount, &category,
		&occurredAt, &note, &tagJSON, &origin, &session, &createdAt); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan entry", err)
	}
	return &ledger.Entry{
		ID:             shared.ID(id),
		Kind:           ledger.EntryKind(kind),
		Amount:         shared.NewMoney(amountMinor, toCurrency(currency)),
		AccountID:      shared.ID(accountID),
		ToAccountID:    idOrEmpty(toAccount),
		CategoryID:     idOrEmpty(category),
		OccurredAt:     toTime(occurredAt),
		Note:           note,
		Tags:           toTags(tagJSON),
		Origin:         ledger.Origin(origin),
		FocusSessionID: idOrEmpty(session),
		CreatedAt:      toTime(createdAt),
	}, nil
}

// entryWhere builds the WHERE clause shared by List, Count, Aggregate and
// AccountBalances, so the four can never disagree about which rows they see.
func entryWhere(f ledger.EntryFilter) (string, []any) {
	var clauses []string
	var args []any
	if !f.From.IsZero() {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, ms(f.From))
	}
	if !f.To.IsZero() {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, ms(f.To))
	}
	if !f.AccountID.IsZero() {
		clauses = append(clauses, "(account_id = ? OR to_account_id = ?)")
		args = append(args, string(f.AccountID), string(f.AccountID))
	}
	if !f.CategoryID.IsZero() {
		clauses = append(clauses, "category_id = ?")
		args = append(args, string(f.CategoryID))
	}
	if len(f.Kind) > 0 {
		holders := make([]string, len(f.Kind))
		for i, k := range f.Kind {
			holders[i] = "?"
			args = append(args, int(k))
		}
		clauses = append(clauses, "kind IN ("+strings.Join(holders, ", ")+")")
	}
	if len(f.Origin) > 0 {
		holders := make([]string, len(f.Origin))
		for i, o := range f.Origin {
			holders[i] = "?"
			args = append(args, int(o))
		}
		clauses = append(clauses, "origin IN ("+strings.Join(holders, ", ")+")")
	}
	if f.Currency != "" {
		clauses = append(clauses, "currency = ?")
		args = append(args, string(f.Currency))
	}
	if f.Keyword != "" {
		clauses = append(clauses, `LOWER(note) LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(strings.ToLower(f.Keyword))+"%")
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
