// Package store is an in-memory implementation of every persistence port: the double the
// application tests run on, and the second implementation the SQLite contract is compared
// with. It enforces the same unique indexes, and WithinTx really rolls back.
package store

import (
	"sort"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// Store holds every entity in memory. The zero value is not usable; call New.
type Store struct {
	tasks      map[shared.ID]*task.Task
	sessions   map[shared.ID]*focus.Session
	accounts   map[shared.ID]*ledger.Account
	categories map[shared.ID]*ledger.Category
	entries    map[shared.ID]*ledger.Entry
	budgets    map[shared.ID]*ledger.Budget
	templates  map[shared.ID]*template.Template
	goals      map[shared.ID]*goal.Goal
	logs       []template.LogEntry
	settings   map[string]string

	// txDepth counts nested transactions; only the outermost snapshots, so a
	// nested commit cannot publish half of an outer rollback.
	txDepth int
}

// New returns an empty store.
func New() *Store {
	return &Store{
		tasks:      make(map[shared.ID]*task.Task),
		sessions:   make(map[shared.ID]*focus.Session),
		accounts:   make(map[shared.ID]*ledger.Account),
		categories: make(map[shared.ID]*ledger.Category),
		entries:    make(map[shared.ID]*ledger.Entry),
		budgets:    make(map[shared.ID]*ledger.Budget),
		templates:  make(map[shared.ID]*template.Template),
		goals:      make(map[shared.ID]*goal.Goal),
		settings:   make(map[string]string),
	}
}

// ── transactions ───────────────────────────────────────────────────────

// WithinTx runs fn and restores the previous state if it returns an error.
func (s *Store) WithinTx(fn func() error) error {
	s.txDepth++
	snapshot := s.snapshot()
	err := fn()
	s.txDepth--
	if err != nil && s.txDepth == 0 {
		s.restore(snapshot)
	}
	return err
}

type snapshot struct {
	tasks      map[shared.ID]*task.Task
	sessions   map[shared.ID]*focus.Session
	accounts   map[shared.ID]*ledger.Account
	categories map[shared.ID]*ledger.Category
	entries    map[shared.ID]*ledger.Entry
	budgets    map[shared.ID]*ledger.Budget
	templates  map[shared.ID]*template.Template
	goals      map[shared.ID]*goal.Goal
	logs       []template.LogEntry
}

func (s *Store) snapshot() snapshot {
	return snapshot{
		tasks:      copyMap(s.tasks),
		sessions:   copyMap(s.sessions),
		accounts:   copyMap(s.accounts),
		categories: copyMap(s.categories),
		entries:    copyMap(s.entries),
		budgets:    copyMap(s.budgets),
		templates:  copyMap(s.templates),
		goals:      copyMap(s.goals),
		logs:       append([]template.LogEntry(nil), s.logs...),
	}
}

// copyMap deep-copies one level of pointers: the rollback snapshot only needs to
// restore field values, and every entity here is a flat struct.
func copyMap[V any](in map[shared.ID]*V) map[shared.ID]*V {
	out := make(map[shared.ID]*V, len(in))
	for k, v := range in {
		c := *v
		out[k] = &c
	}
	return out
}

func (s *Store) restore(sn snapshot) {
	s.tasks, s.sessions, s.accounts, s.categories = sn.tasks, sn.sessions, sn.accounts, sn.categories
	s.entries, s.budgets, s.templates, s.logs = sn.entries, sn.budgets, sn.templates, sn.logs
	s.goals = sn.goals
}

// ── tasks ──────────────────────────────────────────────────────────────

// Create stores a task, enforcing the template occurrence key.
func (r TaskRepo) Create(t *task.Task) error {
	if _, ok := r.tasks[t.ID]; ok {
		return shared.Dupf("task %s", t.ID)
	}
	if t.OriginTemplateID != "" {
		for _, other := range r.tasks {
			if other.OriginTemplateID == t.OriginTemplateID &&
				other.OriginDate.Equal(t.OriginDate) &&
				other.OriginItemSeq == t.OriginItemSeq {
				return shared.Dupf("task occurrence %s/%s/%d", t.OriginTemplateID, t.OriginDate, t.OriginItemSeq)
			}
		}
	}
	r.tasks[t.ID] = copyTask(t)
	return nil
}

// Update replaces a task.
func (r TaskRepo) Update(t *task.Task) error {
	if _, ok := r.tasks[t.ID]; !ok {
		return shared.NotFound("task.not_found", "task not found")
	}
	r.tasks[t.ID] = copyTask(t)
	return nil
}

// Delete removes a task physically.
func (r TaskRepo) Delete(id shared.ID) error {
	if _, ok := r.tasks[id]; !ok {
		return shared.NotFound("task.not_found", "task not found")
	}
	delete(r.tasks, id)
	return nil
}

// Get returns one task.
func (r TaskRepo) Get(id shared.ID) (*task.Task, error) {
	t, ok := r.tasks[id]
	if !ok {
		return nil, shared.NotFound("task.not_found", "task not found")
	}
	return copyTask(t), nil
}

// List returns the tasks matching the filter, sorted by the requested order.
func (r TaskRepo) List(f task.Filter) ([]*task.Task, error) {
	var out []*task.Task
	for _, t := range r.tasks {
		if matchTask(t, f) {
			out = append(out, copyTask(t))
		}
	}
	sortTasks(out, f.Order)
	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return nil, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// Count counts the tasks matching the filter.
func (r TaskRepo) Count(f task.Filter) (int, error) {
	n := 0
	for _, t := range r.tasks {
		if matchTask(t, f) {
			n++
		}
	}
	return n, nil
}

// CountOpen counts todo and doing tasks.
func (r TaskRepo) CountOpen() (int, error) {
	n := 0
	for _, t := range r.tasks {
		if t.Status.IsOpen() {
			n++
		}
	}
	return n, nil
}

// HasFocusHistory reports whether a session references the task.
func (r TaskRepo) HasFocusHistory(id shared.ID) (bool, error) {
	for _, sess := range r.sessions {
		if sess.TaskID == id {
			return true, nil
		}
	}
	return false, nil
}

// ExistsOccurrence reports whether the template already produced this item.
func (r TaskRepo) ExistsOccurrence(templateID shared.ID, day shared.LocalDate, seq int) (bool, error) {
	for _, t := range r.tasks {
		if t.OriginTemplateID == templateID && t.OriginDate.Equal(day) && t.OriginItemSeq == seq {
			return true, nil
		}
	}
	return false, nil
}

func matchTask(t *task.Task, f task.Filter) bool {
	if t.Status == task.StatusArchived && !f.IncludeArchived {
		return false
	}
	if len(f.Statuses) > 0 {
		found := false
		for _, st := range f.Statuses {
			if t.Status == st {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.ParentID != nil && t.ParentID != *f.ParentID {
		return false
	}
	if f.HasDue != nil && t.HasDueDate() != *f.HasDue {
		return false
	}
	if !f.DueFrom.IsZero() && (t.DueAt.IsZero() || t.DueAt.Before(f.DueFrom)) {
		return false
	}
	if !f.DueTo.IsZero() && (t.DueAt.IsZero() || !t.DueAt.Before(f.DueTo)) {
		return false
	}
	if !f.ExcludeDueFrom.IsZero() && !t.DueAt.IsZero() &&
		!t.DueAt.Before(f.ExcludeDueFrom) && t.DueAt.Before(f.ExcludeDueTo) {
		return false
	}
	if !f.DueBefore.IsZero() && (t.DueAt.IsZero() || !t.DueAt.Before(f.DueBefore)) {
		return false
	}
	if f.Search != "" {
		needle := strings.ToLower(f.Search)
		if !strings.Contains(strings.ToLower(t.Title), needle) && !strings.Contains(strings.ToLower(t.Notes), needle) {
			return false
		}
	}
	if f.Tag != "" {
		found := false
		for _, tag := range t.Tags {
			if tag == f.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sortTasks(tasks []*task.Task, order task.Ordering) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		switch order {
		case task.OrderDue:
			if a.DueAt.IsZero() != b.DueAt.IsZero() {
				return !a.DueAt.IsZero()
			}
			if !a.DueAt.Equal(b.DueAt) {
				return a.DueAt.Before(b.DueAt)
			}
		case task.OrderPriority:
			pa, pb := priorityRank(a.Priority), priorityRank(b.Priority)
			if pa != pb {
				return pa < pb
			}
		case task.OrderCreated:
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
		}
		if a.SortKey != b.SortKey {
			return a.SortKey < b.SortKey
		}
		// Ties fall back to the identifier. Every task starts with the same sort
		// key, so without this the order would depend on map iteration and differ
		// from the SQLite adapter on every run.
		return a.ID < b.ID
	})
}

// priorityRank sorts P0 first and unset last.
func priorityRank(p task.Priority) int {
	if p == 0 {
		return 99
	}
	return int(p)
}

func copyTask(t *task.Task) *task.Task {
	cp := *t
	cp.Tags = append([]string(nil), t.Tags...)
	return &cp
}

// ── sessions ───────────────────────────────────────────────────────────

// Create stores a session, enforcing the single-active invariant.
func (r SessionRepo) Create(sess *focus.Session) error {
	if _, ok := r.sessions[sess.ID]; ok {
		return shared.Dupf("session %s", sess.ID)
	}
	if sess.Phase.IsActive() {
		for _, other := range r.sessions {
			if other.Phase.IsActive() {
				return shared.Dupf("active session %s", other.ID)
			}
		}
	}
	r.sessions[sess.ID] = copySession(sess)
	return nil
}

// Update replaces a session.
func (r SessionRepo) Update(sess *focus.Session) error {
	if _, ok := r.sessions[sess.ID]; !ok {
		return shared.NotFound("focus.not_found", "session not found")
	}
	r.sessions[sess.ID] = copySession(sess)
	return nil
}

// Get returns one session.
func (r SessionRepo) Get(id shared.ID) (*focus.Session, error) {
	sess, ok := r.sessions[id]
	if !ok {
		return nil, shared.NotFound("focus.not_found", "session not found")
	}
	return copySession(sess), nil
}

// Active returns the running or paused session, or nil.
func (r SessionRepo) Active() (*focus.Session, error) {
	for _, sess := range r.sessions {
		if sess.Phase.IsActive() {
			return copySession(sess), nil
		}
	}
	return nil, nil
}

// ListByRange returns sessions started in [from, to), newest first.
func (r SessionRepo) ListByRange(from, to time.Time) ([]*focus.Session, error) {
	var out []*focus.Session
	for _, sess := range r.sessions {
		if !sess.StartedAt.Before(from) && sess.StartedAt.Before(to) {
			out = append(out, copySession(sess))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// ListRecent returns the newest sessions.
func (r SessionRepo) ListRecent(limit int) ([]*focus.Session, error) {
	out := make([]*focus.Session, 0, len(r.sessions))
	for _, sess := range r.sessions {
		out = append(out, copySession(sess))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// FocusCountBetween counts completed focus sessions started in [from, to).
func (r SessionRepo) FocusCountBetween(from, to time.Time) (int, error) {
	n := 0
	for _, sess := range r.sessions {
		if sess.Kind != focus.KindFocus || sess.Phase != focus.PhaseDone {
			continue
		}
		if !sess.StartedAt.Before(from) && sess.StartedAt.Before(to) {
			n++
		}
	}
	return n, nil
}

// FocusMinutesBetween sums credited focus minutes in [from, to).
func (r SessionRepo) FocusMinutesBetween(from, to time.Time) (int, error) {
	total := 0
	for _, sess := range r.sessions {
		if sess.Kind != focus.KindFocus || sess.Phase != focus.PhaseDone {
			continue
		}
		if !sess.StartedAt.Before(from) && sess.StartedAt.Before(to) {
			total += sess.CompletedFocusMinutes()
		}
	}
	return total, nil
}

// TaskFocusMinutes sums credited minutes for one task.
func (r SessionRepo) TaskFocusMinutes(taskID shared.ID) (int, error) {
	total := 0
	for _, sess := range r.sessions {
		if sess.TaskID == taskID {
			total += sess.CompletedFocusMinutes()
		}
	}
	return total, nil
}

func copySession(sess *focus.Session) *focus.Session {
	cp := *sess
	return &cp
}

// ── accounts ───────────────────────────────────────────────────────────

// Create stores an account, enforcing the builtin key uniqueness.
func (r AccountRepo) Create(a *ledger.Account) error {
	if _, ok := r.accounts[a.ID]; ok {
		return shared.Dupf("account %s", a.ID)
	}
	if a.BuiltinKey != "" {
		for _, other := range r.accounts {
			if other.BuiltinKey == a.BuiltinKey {
				return shared.Dupf("account builtin %s", a.BuiltinKey)
			}
		}
	}
	cp := *a
	r.accounts[a.ID] = &cp
	return nil
}

// Update replaces an account.
func (r AccountRepo) Update(a *ledger.Account) error {
	if _, ok := r.accounts[a.ID]; !ok {
		return shared.NotFound("ledger.account.not_found", "account not found")
	}
	cp := *a
	r.accounts[a.ID] = &cp
	return nil
}

// Delete removes an account.
func (r AccountRepo) Delete(id shared.ID) error {
	if _, ok := r.accounts[id]; !ok {
		return shared.NotFound("ledger.account.not_found", "account not found")
	}
	delete(r.accounts, id)
	return nil
}

// List returns accounts ordered by sort order then name.
func (r AccountRepo) List(includeArchived bool) ([]*ledger.Account, error) {
	out := make([]*ledger.Account, 0, len(r.accounts))
	for _, a := range r.accounts {
		if a.Archived && !includeArchived {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Get returns one account.
func (r AccountRepo) Get(id shared.ID) (*ledger.Account, error) {
	a, ok := r.accounts[id]
	if !ok {
		return nil, shared.NotFound("ledger.account.not_found", "account not found")
	}
	cp := *a
	return &cp, nil
}

// CountEntries counts entries touching the account, both sides of a transfer.
func (r AccountRepo) CountEntries(id shared.ID) (int, error) {
	n := 0
	for _, e := range r.entries {
		if e.AccountID == id || e.ToAccountID == id {
			n++
		}
	}
	return n, nil
}

// ── categories ─────────────────────────────────────────────────────────

// Create stores a category.
func (r CategoryRepo) Create(c *ledger.Category) error {
	if _, ok := r.categories[c.ID]; ok {
		return shared.Dupf("category %s", c.ID)
	}
	if c.BuiltinKey != "" {
		for _, other := range r.categories {
			if other.BuiltinKey == c.BuiltinKey {
				return shared.Dupf("category builtin %s", c.BuiltinKey)
			}
		}
	}
	cp := *c
	r.categories[c.ID] = &cp
	return nil
}

// Update replaces a category.
func (r CategoryRepo) Update(c *ledger.Category) error {
	if _, ok := r.categories[c.ID]; !ok {
		return shared.NotFound("ledger.category.not_found", "category not found")
	}
	cp := *c
	r.categories[c.ID] = &cp
	return nil
}

// Delete removes a category.
func (r CategoryRepo) Delete(id shared.ID) error {
	if _, ok := r.categories[id]; !ok {
		return shared.NotFound("ledger.category.not_found", "category not found")
	}
	delete(r.categories, id)
	return nil
}

// List returns categories ordered by direction, sort order and name.
func (r CategoryRepo) List() ([]*ledger.Category, error) {
	out := make([]*ledger.Category, 0, len(r.categories))
	for _, c := range r.categories {
		cp := *c
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Direction != out[j].Direction {
			return out[i].Direction < out[j].Direction
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Get returns one category.
func (r CategoryRepo) Get(id shared.ID) (*ledger.Category, error) {
	c, ok := r.categories[id]
	if !ok {
		return nil, shared.NotFound("ledger.category.not_found", "category not found")
	}
	cp := *c
	return &cp, nil
}

// CountEntries counts entries referencing the category.
func (r CategoryRepo) CountEntries(id shared.ID) (int, error) {
	n := 0
	for _, e := range r.entries {
		if e.CategoryID == id {
			n++
		}
	}
	return n, nil
}

// CountChildren counts sub-categories.
func (r CategoryRepo) CountChildren(id shared.ID) (int, error) {
	n := 0
	for _, c := range r.categories {
		if c.ParentID == id {
			n++
		}
	}
	return n, nil
}

// ── entries ────────────────────────────────────────────────────────────

// Create stores an entry, enforcing the focus booking uniqueness.
func (r EntryRepo) Create(e *ledger.Entry) error {
	if _, ok := r.entries[e.ID]; ok {
		return shared.Dupf("entry %s", e.ID)
	}
	if e.FocusSessionID != "" {
		for _, other := range r.entries {
			if other.FocusSessionID == e.FocusSessionID {
				return shared.Dupf("entry focus session %s", e.FocusSessionID)
			}
		}
	}
	r.entries[e.ID] = copyEntry(e)
	return nil
}

// Update replaces an entry.
func (r EntryRepo) Update(e *ledger.Entry) error {
	if _, ok := r.entries[e.ID]; !ok {
		return shared.NotFound("ledger.entry.not_found", "entry not found")
	}
	r.entries[e.ID] = copyEntry(e)
	return nil
}

// Delete removes an entry.
func (r EntryRepo) Delete(id shared.ID) error {
	if _, ok := r.entries[id]; !ok {
		return shared.NotFound("ledger.entry.not_found", "entry not found")
	}
	delete(r.entries, id)
	return nil
}

// Get returns one entry.
func (r EntryRepo) Get(id shared.ID) (*ledger.Entry, error) {
	e, ok := r.entries[id]
	if !ok {
		return nil, shared.NotFound("ledger.entry.not_found", "entry not found")
	}
	return copyEntry(e), nil
}

// List returns the entries matching the filter.
func (r EntryRepo) List(f ledger.EntryFilter) ([]*ledger.Entry, error) {
	var out []*ledger.Entry
	for _, e := range r.entries {
		if matchEntry(e, f) {
			out = append(out, copyEntry(e))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			if f.NewestFirst {
				return out[i].OccurredAt.After(out[j].OccurredAt)
			}
			return out[i].OccurredAt.Before(out[j].OccurredAt)
		}
		return out[i].ID < out[j].ID
	})
	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return nil, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// Count counts the entries matching the filter.
func (r EntryRepo) Count(f ledger.EntryFilter) (int, error) {
	n := 0
	for _, e := range r.entries {
		if matchEntry(e, f) {
			n++
		}
	}
	return n, nil
}

// Aggregate groups the matching entries.
func (r EntryRepo) Aggregate(f ledger.EntryFilter, groupBy ledger.AggregateGroup) ([]ledger.AggregateRow, error) {
	sums := make(map[string]int64)
	counts := make(map[string]int)
	currency := make(map[string]shared.Currency)
	for _, e := range r.entries {
		if !matchEntry(e, f) {
			continue
		}
		key := ""
		switch groupBy {
		case ledger.GroupDay:
			key = shared.DateOf(e.OccurredAt.Local()).String()
		case ledger.GroupMonth:
			key = shared.MonthOf(e.OccurredAt.Local()).String()
		case ledger.GroupCategory:
			key = string(e.CategoryID)
		case ledger.GroupAccount:
			key = string(e.AccountID)
		}
		sums[key] += e.SignedAmount()
		counts[key]++
		currency[key] = e.Amount.Currency()
	}
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ledger.AggregateRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, ledger.AggregateRow{Key: k, Sum: shared.NewMoney(sums[k], currency[k]), Count: counts[k], Currency: currency[k]})
	}
	return out, nil
}

// SumByOrigin totals entries of one origin and kind in a period.
func (r EntryRepo) SumByOrigin(origin ledger.Origin, kind ledger.EntryKind, from, to time.Time, currency shared.Currency) (shared.Money, error) {
	var total int64
	for _, e := range r.entries {
		if e.Origin != origin || e.Kind != kind || e.Amount.Currency() != currency {
			continue
		}
		if e.OccurredAt.Before(from) || !e.OccurredAt.Before(to) {
			continue
		}
		total += e.Amount.Minor()
	}
	return shared.NewMoney(total, currency), nil
}

// ExistsFocusSession reports whether a session already produced an entry.
func (r EntryRepo) ExistsFocusSession(sessionID shared.ID) (bool, error) {
	for _, e := range r.entries {
		if e.FocusSessionID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

// AccountBalances returns each account's net movement, both transfer sides.
func (r EntryRepo) AccountBalances(f ledger.EntryFilter) ([]ledger.AccountBalanceRow, error) {
	sums := make(map[shared.ID]int64)
	currencies := make(map[shared.ID]shared.Currency)
	for _, e := range r.entries {
		if !matchEntry(e, f) {
			continue
		}
		sums[e.AccountID] += e.SignedAmount()
		currencies[e.AccountID] = e.Amount.Currency()
		if e.Kind == ledger.EntryTransfer && !e.ToAccountID.IsZero() {
			sums[e.ToAccountID] += e.Amount.Minor()
			currencies[e.ToAccountID] = e.Amount.Currency()
		}
	}
	ids := make([]shared.ID, 0, len(sums))
	for id := range sums {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]ledger.AccountBalanceRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, ledger.AccountBalanceRow{AccountID: id, Sum: sums[id], Currency: currencies[id]})
	}
	return out, nil
}

func matchEntry(e *ledger.Entry, f ledger.EntryFilter) bool {
	if !f.From.IsZero() && e.OccurredAt.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && !e.OccurredAt.Before(f.To) {
		return false
	}
	if f.AccountID != "" && e.AccountID != f.AccountID && e.ToAccountID != f.AccountID {
		return false
	}
	if f.CategoryID != "" && e.CategoryID != f.CategoryID {
		return false
	}
	if len(f.Kind) > 0 {
		found := false
		for _, k := range f.Kind {
			if e.Kind == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.Origin) > 0 {
		found := false
		for _, o := range f.Origin {
			if e.Origin == o {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.Currency != "" && e.Amount.Currency() != f.Currency {
		return false
	}
	if f.Keyword != "" {
		needle := strings.ToLower(f.Keyword)
		if !strings.Contains(strings.ToLower(e.Note), needle) {
			return false
		}
	}
	return true
}

func copyEntry(e *ledger.Entry) *ledger.Entry {
	cp := *e
	cp.Tags = append([]string(nil), e.Tags...)
	return &cp
}

// ── budgets ────────────────────────────────────────────────────────────

// Upsert stores a budget, replacing the one for the same category and month.
func (r BudgetRepo) Upsert(b *ledger.Budget) error {
	for id, other := range r.budgets {
		if other.CategoryID == b.CategoryID && other.Month == b.Month && id != b.ID {
			delete(r.budgets, id)
		}
	}
	cp := *b
	r.budgets[b.ID] = &cp
	return nil
}

// Delete removes a budget.
func (r BudgetRepo) Delete(id shared.ID) error {
	if _, ok := r.budgets[id]; !ok {
		return shared.NotFound("ledger.budget.not_found", "budget not found")
	}
	delete(r.budgets, id)
	return nil
}

// ListByMonth returns the budgets of one month.
func (r BudgetRepo) ListByMonth(m shared.Month) ([]*ledger.Budget, error) {
	out := make([]*ledger.Budget, 0, len(r.budgets))
	for _, b := range r.budgets {
		if b.Month == m {
			cp := *b
			out = append(out, &cp)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListAll returns every budget.
func (r BudgetRepo) ListAll() ([]*ledger.Budget, error) {
	out := make([]*ledger.Budget, 0, len(r.budgets))
	for _, b := range r.budgets {
		cp := *b
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month.String() < out[j].Month.String()
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get returns the budget of one category and month.
func (r BudgetRepo) Get(categoryID shared.ID, m shared.Month) (*ledger.Budget, error) {
	for _, b := range r.budgets {
		if b.CategoryID == categoryID && b.Month == m {
			cp := *b
			return &cp, nil
		}
	}
	return nil, shared.NotFound("ledger.budget.not_found", "budget not found")
}

// ── templates ──────────────────────────────────────────────────────────

// Save creates or replaces a template.
func (r TemplateRepo) Save(t *template.Template) error {
	r.templates[t.ID] = copyTemplate(t)
	return nil
}

// Delete removes a template.
func (r TemplateRepo) Delete(id shared.ID) error {
	if _, ok := r.templates[id]; !ok {
		return shared.NotFound("template.not_found", "template not found")
	}
	delete(r.templates, id)
	return nil
}

// Get returns one template.
func (r TemplateRepo) Get(id shared.ID) (*template.Template, error) {
	t, ok := r.templates[id]
	if !ok {
		return nil, shared.NotFound("template.not_found", "template not found")
	}
	return copyTemplate(t), nil
}

// List returns every template.
func (r TemplateRepo) List() ([]*template.Template, error) {
	out := make([]*template.Template, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, copyTemplate(t))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListEnabled returns the enabled templates.
func (r TemplateRepo) ListEnabled() ([]*template.Template, error) {
	all, err := r.List()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, t := range all {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return out, nil
}

// MarkExpanded appends a log row and stamps the template.
func (r TemplateRepo) MarkExpanded(id shared.ID, day shared.LocalDate, entry template.LogEntry) error {
	t, ok := r.templates[id]
	if !ok {
		return shared.NotFound("template.not_found", "template not found")
	}
	entry.TemplateID = id
	if entry.Day.IsZero() {
		entry.Day = day
	}
	r.logs = append(r.logs, entry)
	cp := *t
	cp.UpdatedAt = entry.At
	r.templates[id] = &cp
	return nil
}

// RecentLogs returns the newest log rows.
func (r TemplateRepo) RecentLogs(limit int) ([]template.LogEntry, error) {
	// Reversed first so that two runs in the same millisecond come back newest
	// first, matching the SQLite adapter's "created_at DESC, id DESC".
	out := make([]template.LogEntry, len(r.logs))
	for i, entry := range r.logs {
		out[len(r.logs)-1-i] = entry
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func copyTemplate(t *template.Template) *template.Template {
	cp := *t
	cp.Task.Tags = append([]string(nil), t.Task.Tags...)
	cp.Rule.Weekdays = append([]time.Weekday(nil), t.Rule.Weekdays...)
	cp.Rule.Dates = append([]shared.LocalDate(nil), t.Rule.Dates...)
	return &cp
}

// ── goals ──────────────────────────────────────────────────────────────

// Save creates or replaces a goal.
func (r GoalRepo) Save(g *goal.Goal) error {
	cp := *g
	r.goals[g.ID] = &cp
	return nil
}

// Delete removes a goal.
func (r GoalRepo) Delete(id shared.ID) error {
	if _, ok := r.goals[id]; !ok {
		return shared.NotFound("goal.not_found", "goal not found")
	}
	delete(r.goals, id)
	return nil
}

// Get returns one goal.
func (r GoalRepo) Get(id shared.ID) (*goal.Goal, error) {
	g, ok := r.goals[id]
	if !ok {
		return nil, shared.NotFound("goal.not_found", "goal not found")
	}
	cp := *g
	return &cp, nil
}

// List returns every goal, the nearest deadline first, then the one set first.
func (r GoalRepo) List() ([]*goal.Goal, error) {
	out := make([]*goal.Goal, 0, len(r.goals))
	for _, g := range r.goals {
		cp := *g
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].End.Equal(out[j].End) {
			return out[i].End.Before(out[j].End)
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ── settings ───────────────────────────────────────────────────────────

// Get returns a settings value.
func (r SettingsRepo) Get(key string) (string, error) {
	v, ok := r.settings[key]
	if !ok {
		return "", shared.ErrNotFound
	}
	return v, nil
}

// Set stores a settings value.
func (r SettingsRepo) Set(key, value string) error {
	r.settings[key] = value
	return nil
}

// Typed facades: the ports share method names with different entity types, so each one
// gets its own struct around the shared data.

// TaskRepo adapts the store to task.Repository.
type TaskRepo struct{ *Store }

// SessionRepo adapts the store to focus.Repository.
type SessionRepo struct{ *Store }

// AccountRepo adapts the store to ledger.AccountRepository.
type AccountRepo struct{ *Store }

// CategoryRepo adapts the store to ledger.CategoryRepository.
type CategoryRepo struct{ *Store }

// EntryRepo adapts the store to ledger.EntryRepository.
type EntryRepo struct{ *Store }

// BudgetRepo adapts the store to ledger.BudgetRepository.
type BudgetRepo struct{ *Store }

// TemplateRepo adapts the store to template.Repository.
type TemplateRepo struct{ *Store }

// GoalRepo adapts the store to goal.Repository.
type GoalRepo struct{ *Store }

// SettingsRepo adapts the store to settings.Store.
type SettingsRepo struct{ *Store }

// Tasks returns the task repository.
func (s *Store) Tasks() TaskRepo { return TaskRepo{s} }

// Sessions returns the focus repository.
func (s *Store) Sessions() SessionRepo { return SessionRepo{s} }

// Accounts returns the account repository.
func (s *Store) Accounts() AccountRepo { return AccountRepo{s} }

// Categories returns the category repository.
func (s *Store) Categories() CategoryRepo { return CategoryRepo{s} }

// Entries returns the entry repository.
func (s *Store) Entries() EntryRepo { return EntryRepo{s} }

// Budgets returns the budget repository.
func (s *Store) Budgets() BudgetRepo { return BudgetRepo{s} }

// Templates returns the template repository.
func (s *Store) Templates() TemplateRepo { return TemplateRepo{s} }

// Goals returns the goal repository.
func (s *Store) Goals() GoalRepo { return GoalRepo{s} }

// Settings returns the settings store.
func (s *Store) Settings() SettingsRepo { return SettingsRepo{s} }
