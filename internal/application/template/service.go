// Package template implements the todo-template use cases and the idempotent daily
// expansion. The unique index on the occurrence key is the final authority; the check
// here only avoids pointless writes.
package template

import (
	"errors"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	domain "github.com/iotus3/simplelife/app/internal/domain/template"
)

// occurrenceSeq is the origin sequence a template writes. A template produces one
// task per day, so the value is constant; the column stays because the unique
// index that makes expansion idempotent is keyed on it.
const occurrenceSeq = 0

// Service is the template application service.
type Service struct {
	repo  domain.Repository
	tasks task.Repository
	tx    shared.TxRunner
	clock shared.Clock
	ids   shared.IDGen
}

// New wires the service.
func New(repo domain.Repository, tasks task.Repository, tx shared.TxRunner, clock shared.Clock, ids shared.IDGen) *Service {
	return &Service{repo: repo, tasks: tasks, tx: tx, clock: clock, ids: ids}
}

// SaveInput is the form data for a template.
type SaveInput struct {
	ID      shared.ID // empty = create
	Name    string
	Enabled bool
	Rule    domain.TriggerRule
	Task    domain.Task
}

// Save creates or updates a template. The caller is expected to expand today
// afterwards so a newly added template shows up immediately instead of tomorrow.
func (s *Service) Save(in SaveInput) (*domain.Template, error) {
	now := s.clock.Now()
	var out *domain.Template
	err := s.tx.WithinTx(func() error {
		tpl := &domain.Template{ID: in.ID}
		if in.ID.IsZero() {
			tpl.ID = s.ids.New()
			tpl.CreatedAt = now
		} else {
			existing, err := s.repo.Get(in.ID)
			if err != nil {
				return err
			}
			tpl.CreatedAt = existing.CreatedAt
		}
		tpl.Name = in.Name
		tpl.Enabled = in.Enabled
		tpl.Rule = in.Rule
		tpl.UpdatedAt = now
		if err := tpl.SetTask(in.Task, now); err != nil {
			return err
		}
		if err := s.repo.Save(tpl); err != nil {
			return err
		}
		out = tpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a template. Tasks it already produced stay: they are independent
// entities the user may have edited.
func (s *Service) Delete(id shared.ID) error {
	return s.tx.WithinTx(func() error { return s.repo.Delete(id) })
}

// List returns every template.
func (s *Service) List() ([]*domain.Template, error) { return s.repo.List() }

// Get loads one template.
func (s *Service) Get(id shared.ID) (*domain.Template, error) { return s.repo.Get(id) }

// Failure records one template that could not be materialised, so the UI can say
// "3 created, 1 skipped, 1 failed" instead of pretending everything worked.
type Failure struct {
	TemplateID shared.ID
	Template   string
	Seq        int
	Code       string
}

// ExpandResult reports what one expansion run did.
type ExpandResult struct {
	Day     shared.LocalDate
	Created int
	Skipped int
	Failed  []Failure
}

// Expand materialises the task of every template that fires on day. A deleted generated
// task stays deleted, editing a template never rewrites produced tasks, and disabling
// stops future days only.
func (s *Service) Expand(day shared.LocalDate) (ExpandResult, error) {
	res := ExpandResult{Day: day}
	if day.IsZero() {
		return res, shared.Invalid("template.expand.day", "", "expansion day is required")
	}
	templates, err := s.repo.ListEnabled()
	if err != nil {
		return res, err
	}
	now := s.clock.Now()
	for _, tpl := range templates {
		if !tpl.Rule.Matches(day) {
			continue
		}
		createdForTemplate := 0
		// One template writes one task: the occurrence key is the template and the
		// day, so the unique index still refuses a second run.
		exists, err := s.tasks.ExistsOccurrence(tpl.ID, day, occurrenceSeq)
		if err != nil {
			return res, err
		}
		if exists {
			res.Skipped++
			continue
		}
		newTask, err := task.NewFromTemplate(
			s.ids.New(),
			task.FromTemplateSpec{
				Title:         tpl.Task.Title,
				Notes:         tpl.Task.Notes,
				Priority:      tpl.Task.Priority,
				Estimate:      tpl.Task.Estimate,
				FocusMinutes:  tpl.Task.FocusMinutes,
				DueOffsetDays: tpl.Task.DueOffsetDays,
				Tags:          tpl.Task.Tags,
			},
			task.TemplateOrigin{TemplateID: tpl.ID, Day: day, Seq: occurrenceSeq},
			now,
		)
		if err != nil {
			res.Failed = append(res.Failed, Failure{TemplateID: tpl.ID, Template: tpl.Name, Seq: occurrenceSeq, Code: shared.Code(err)})
			continue
		}
		if err := s.tasks.Create(newTask); err != nil {
			if errors.Is(err, shared.ErrDuplicate) {
				// The unique index caught a race: this occurrence exists.
				res.Skipped++
				continue
			}
			res.Failed = append(res.Failed, Failure{TemplateID: tpl.ID, Template: tpl.Name, Seq: occurrenceSeq, Code: shared.Code(err)})
			continue
		}
		res.Created++
		createdForTemplate++
		if createdForTemplate > 0 {
			// Only for the "last generated" line in the UI; never authoritative.
			_ = s.repo.MarkExpanded(tpl.ID, day, domain.LogEntry{
				TemplateID: tpl.ID,
				Day:        day,
				Created:    createdForTemplate,
				Failed:     len(res.Failed),
				At:         now,
			})
		}
	}
	return res, nil
}

// PreviewItem is one task a template would create on a given day.
type PreviewItem struct {
	TemplateName string
	Title        string
	DueDate      shared.LocalDate
	Exists       bool
}

// Preview answers "what would today generate?" without writing anything, which is
// what the template editor shows while the user tunes the rule.
func (s *Service) Preview(day shared.LocalDate) ([]PreviewItem, error) {
	templates, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	var out []PreviewItem
	for _, tpl := range templates {
		if !tpl.Enabled || !tpl.Rule.Matches(day) {
			continue
		}
		exists, err := s.tasks.ExistsOccurrence(tpl.ID, day, occurrenceSeq)
		if err != nil {
			return nil, err
		}
		out = append(out, PreviewItem{
			TemplateName: tpl.Name,
			Title:        tpl.Task.Title,
			DueDate:      day.AddDays(tpl.Task.DueOffsetDays),
			Exists:       exists,
		})
	}
	return out, nil
}

// NextRun returns the next day on or after from on which the template fires, or a
// zero date when it never will (for example a one-off date in the past).
func NextRun(tpl *domain.Template, from shared.LocalDate) shared.LocalDate {
	if tpl == nil || !tpl.Enabled {
		return shared.LocalDate{}
	}
	// A year of days is enough for every supported rule; a one-off date list that
	// has already passed simply reports "never".
	for i := 0; i < 366; i++ {
		day := from.AddDays(i)
		if tpl.Rule.Matches(day) {
			return day
		}
	}
	return shared.LocalDate{}
}

// RecentLogs returns the newest expansion log rows.
func (s *Service) RecentLogs(limit int) ([]domain.LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.RecentLogs(limit)
}

// LastExpanded is a convenience for the template list: the most recent log row for
// one template.
func (s *Service) LastExpanded(tplID shared.ID) (domain.LogEntry, bool, error) {
	logs, err := s.repo.RecentLogs(50)
	if err != nil {
		return domain.LogEntry{}, false, err
	}
	for _, l := range logs {
		if l.TemplateID == tplID {
			return l, true, nil
		}
	}
	return domain.LogEntry{}, false, nil
}
