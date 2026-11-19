// Package export materialises a Plan into sheets of strings; the encoders write bytes.
package export

import (
	"strconv"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"

	domain "github.com/iotus3/simplelife/app/internal/domain/export"
)

// Labels supplies the localised header and sheet names. The application layer
// cannot know the interface language, so the presentation layer passes this in.
type Labels interface {
	// Sheet is the human-readable name of a table ("待办" / "Tasks").
	Sheet(table string) string
	// Column is the header of one column.
	Column(table, column string) string
}

// Service builds sheets from the stores.
type Service struct {
	tasks      task.Repository
	sessions   focus.Repository
	entries    ledger.EntryRepository
	accounts   ledger.AccountRepository
	categories ledger.CategoryRepository
	budgets    ledger.BudgetRepository
	templates  template.Repository
	clock      shared.Clock
}

// New wires the service.
func New(
	tasks task.Repository,
	sessions focus.Repository,
	entries ledger.EntryRepository,
	accounts ledger.AccountRepository,
	categories ledger.CategoryRepository,
	budgets ledger.BudgetRepository,
	templates template.Repository,
	clock shared.Clock,
) *Service {
	return &Service{
		tasks: tasks, sessions: sessions, entries: entries, accounts: accounts,
		categories: categories, budgets: budgets, templates: templates, clock: clock,
	}
}

// Build validates the plan and materialises every selected table.
func (s *Service) Build(plan domain.Plan, labels Labels) ([]domain.Sheet, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	sheets := make([]domain.Sheet, 0, len(plan.Tables))
	for _, sel := range plan.Tables {
		table, ok := domain.TableByKey(sel.Key)
		if !ok {
			return nil, shared.Invalid("export.table.unknown", "tables", "unknown table: "+sel.Key)
		}
		rows, err := s.rows(plan, sel)
		if err != nil {
			return nil, err
		}
		headers := make([]string, len(sel.Columns))
		for i, col := range sel.Columns {
			headers[i] = labels.Column(sel.Key, col)
		}
		sheets = append(sheets, domain.Sheet{
			Name:     labels.Sheet(sel.Key),
			FileBase: table.FileBase,
			Headers:  headers,
			Rows:     rows,
		})
	}
	return sheets, nil
}

// rows materialises one table, projecting and formatting the selected columns.
func (s *Service) rows(plan domain.Plan, sel domain.TableSelection) ([][]string, error) {
	project := func(all map[string]string) []string {
		out := make([]string, 0, len(sel.Columns))
		for _, key := range sel.Columns {
			out = append(out, all[key])
		}
		return out
	}
	switch sel.Key {
	case "tasks":
		tasks, err := s.tasks.List(task.Filter{IncludeArchived: plan.IncludeArchived})
		if err != nil {
			return nil, err
		}
		out := make([][]string, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, project(map[string]string{
				"id":                 string(t.ID),
				"title":              t.Title,
				"status":             t.Status.String(),
				"priority":           strconv.Itoa(int(t.Priority)),
				"due_date":           t.DueDate().String(),
				"estimate_pomodoros": strconv.Itoa(t.EstimatePomodoros),
				"focus_minutes":      strconv.Itoa(t.FocusMinutes),
				"done_pomodoros":     strconv.Itoa(t.DonePomodoros),
				"tags":               strings.Join(t.Tags, ","),
				"notes":              t.Notes,
				"created_at":         formatTime(t.CreatedAt),
				"completed_at":       formatTime(t.CompletedAt),
				"template_id":        string(t.OriginTemplateID),
			}))
		}
		return out, nil
	case "sessions":
		sessions, err := s.sessions.ListByRange(rangeBounds(plan.Range))
		if err != nil {
			return nil, err
		}
		titles, err := s.taskTitles()
		if err != nil {
			return nil, err
		}
		out := make([][]string, 0, len(sessions))
		for _, sess := range sessions {
			out = append(out, project(map[string]string{
				"id":              string(sess.ID),
				"kind":            sess.Kind.String(),
				"phase":           sess.Phase.String(),
				"title":           titles[sess.TaskID],
				"started_at":      formatTime(sess.StartedAt),
				"ended_at":        formatTime(sess.EndedAt),
				"planned_minutes": strconv.Itoa(int(sess.Planned / time.Minute)),
				"elapsed_minutes": strconv.Itoa(int(sess.Elapsed(sess.EndedAt) / time.Minute)),
				"interruptions":   strconv.Itoa(sess.Interruptions),
				"note":            sess.Note,
			}))
		}
		return out, nil
	case "entries":
		filter := ledger.EntryFilter{From: plan.Range.Start(), To: plan.Range.End(), NewestFirst: true, Currency: plan.Currency}
		entries, err := s.entries.List(filter)
		if err != nil {
			return nil, err
		}
		accounts, err := s.accountNames()
		if err != nil {
			return nil, err
		}
		categories, err := s.categoryNames()
		if err != nil {
			return nil, err
		}
		out := make([][]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, project(map[string]string{
				"id":          string(e.ID),
				"kind":        e.Kind.String(),
				"amount":      formatMoney(e.Amount),
				"currency":    string(e.Amount.Currency()),
				"account":     accounts[e.AccountID],
				"to_account":  accounts[e.ToAccountID],
				"category":    categories[e.CategoryID],
				"occurred_at": formatTime(e.OccurredAt),
				"origin":      e.Origin.String(),
				"note":        e.Note,
				"tags":        strings.Join(e.Tags, ","),
			}))
		}
		return out, nil
	case "accounts":
		accounts, err := s.accounts.List(true)
		if err != nil {
			return nil, err
		}
		rows, err := s.entries.AccountBalances(ledger.EntryFilter{})
		if err != nil {
			return nil, err
		}
		delta := make(map[shared.ID]int64, len(rows))
		for _, r := range rows {
			delta[r.AccountID] += r.Sum
		}
		out := make([][]string, 0, len(accounts))
		for _, a := range accounts {
			out = append(out, project(map[string]string{
				"name":     a.Name,
				"kind":     a.Kind.String(),
				"currency": string(a.Currency()),
				"balance":  formatMoney(shared.NewMoney(a.Initial.Minor()+delta[a.ID], a.Currency())),
				"archived": strconv.FormatBool(a.Archived),
			}))
		}
		return out, nil
	case "categories":
		categories, err := s.categories.List()
		if err != nil {
			return nil, err
		}
		names := make(map[shared.ID]string, len(categories))
		for _, c := range categories {
			names[c.ID] = c.Name
		}
		out := make([][]string, 0, len(categories))
		for _, c := range categories {
			out = append(out, project(map[string]string{
				"name":      c.Name,
				"direction": c.Direction.String(),
				"parent":    names[c.ParentID],
			}))
		}
		return out, nil
	case "budgets":
		budgets, err := s.budgets.ListAll()
		if err != nil {
			return nil, err
		}
		categories, err := s.categoryNames()
		if err != nil {
			return nil, err
		}
		out := make([][]string, 0, len(budgets))
		for _, b := range budgets {
			out = append(out, project(map[string]string{
				"month":    b.Month.String(),
				"category": categories[b.CategoryID],
				"limit":    formatMoney(b.Limit),
				"currency": string(b.Limit.Currency()),
			}))
		}
		return out, nil
	case "templates":
		templates, err := s.templates.List()
		if err != nil {
			return nil, err
		}
		out := make([][]string, 0, len(templates))
		for _, tpl := range templates {
			out = append(out, project(map[string]string{
				"name":          tpl.Name,
				"enabled":       strconv.FormatBool(tpl.Enabled),
				"rule":          describeRule(tpl.Rule),
				"task":          tpl.Task.Title,
				"notes":         tpl.Task.Notes,
				"estimate":      strconv.Itoa(tpl.Task.Estimate),
				"focus_minutes": strconv.Itoa(tpl.Task.FocusMinutes),
				"due_offset":    strconv.Itoa(tpl.Task.DueOffsetDays),
			}))
		}
		return out, nil
	}
	return nil, shared.Invalid("export.table.unknown", "tables", "unknown table: "+sel.Key)
}

func (s *Service) taskTitles() (map[shared.ID]string, error) {
	tasks, err := s.tasks.List(task.Filter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	out := make(map[shared.ID]string, len(tasks))
	for _, t := range tasks {
		out[t.ID] = t.Title
	}
	return out, nil
}

func (s *Service) accountNames() (map[shared.ID]string, error) {
	accounts, err := s.accounts.List(true)
	if err != nil {
		return nil, err
	}
	out := make(map[shared.ID]string, len(accounts))
	for _, a := range accounts {
		out[a.ID] = a.Name
	}
	return out, nil
}

func (s *Service) categoryNames() (map[shared.ID]string, error) {
	categories, err := s.categories.List()
	if err != nil {
		return nil, err
	}
	out := make(map[shared.ID]string, len(categories))
	for _, c := range categories {
		out[c.ID] = c.Name
	}
	return out, nil
}

// rangeBounds widens an unbounded range so the store can take plain instants.
func rangeBounds(r shared.DateRange) (time.Time, time.Time) {
	from := r.Start()
	if from.IsZero() {
		from = time.Unix(0, 0)
	}
	to := r.End()
	if to.IsZero() {
		to = time.Date(2200, time.January, 1, 0, 0, 0, 0, time.Local)
	}
	return from, to
}

// formatTime writes ISO-like local time. It is never localised: an export is read
// by scripts as often as by people, and "Nov 17, 2026" sorts wrong (ADR-013).
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// formatMoney writes the canonical, currency-less decimal ("-12.50"); the currency
// travels in its own column so a spreadsheet can format it.
func formatMoney(m shared.Money) string { return m.String() }

// describeRule renders a trigger rule as a stable, language-neutral string, so
// re-importing the export later does not depend on the interface language.
func describeRule(r template.TriggerRule) string {
	switch r.Kind {
	case template.TriggerDaily:
		return "daily"
	case template.TriggerWeekdays:
		days := make([]string, 0, len(r.Weekdays))
		for _, d := range r.Weekdays {
			days = append(days, strconv.Itoa(int(d)))
		}
		return "weekdays:" + strings.Join(days, ",")
	case template.TriggerMonthly:
		return "monthly:" + strconv.Itoa(r.DayOfMonth)
	case template.TriggerSpecificDates:
		dates := make([]string, 0, len(r.Dates))
		for _, d := range r.Dates {
			dates = append(dates, d.String())
		}
		return "dates:" + strings.Join(dates, ",")
	case template.TriggerEveryNDays:
		return "every_n_days:" + strconv.Itoa(r.EveryNDays) + ":from:" + r.Anchor.String()
	}
	return "unknown"
}

// Today is the date stamp the suggested file name uses.
func (s *Service) Today() shared.LocalDate { return shared.DateOf(s.clock.Now()) }
