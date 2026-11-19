package sqlite

import (
	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// Typed facades: the ports share method names with different entity types, so each one gets
// its own struct around the shared connection.
type (
	// TaskRepo adapts the database to task.Repository.
	TaskRepo struct{ *DB }
	// SessionRepo adapts the database to focus.Repository.
	SessionRepo struct{ *DB }
	// AccountRepo adapts the database to ledger.AccountRepository.
	AccountRepo struct{ *DB }
	// CategoryRepo adapts the database to ledger.CategoryRepository.
	CategoryRepo struct{ *DB }
	// EntryRepo adapts the database to ledger.EntryRepository.
	EntryRepo struct{ *DB }
	// BudgetRepo adapts the database to ledger.BudgetRepository.
	BudgetRepo struct{ *DB }
	// TemplateRepo adapts the database to template.Repository.
	TemplateRepo struct{ *DB }
	// GoalRepo adapts the database to goal.Repository.
	GoalRepo struct{ *DB }
	// SettingsRepo adapts the database to settings.Store.
	SettingsRepo struct{ *DB }
)

// Tasks returns the task repository.
func (d *DB) Tasks() TaskRepo { return TaskRepo{d} }

// Sessions returns the focus repository.
func (d *DB) Sessions() SessionRepo { return SessionRepo{d} }

// Accounts returns the account repository.
func (d *DB) Accounts() AccountRepo { return AccountRepo{d} }

// Categories returns the category repository.
func (d *DB) Categories() CategoryRepo { return CategoryRepo{d} }

// Entries returns the entry repository.
func (d *DB) Entries() EntryRepo { return EntryRepo{d} }

// Budgets returns the budget repository.
func (d *DB) Budgets() BudgetRepo { return BudgetRepo{d} }

// Templates returns the template repository.
func (d *DB) Templates() TemplateRepo { return TemplateRepo{d} }

// Goals returns the goal repository.
func (d *DB) Goals() GoalRepo { return GoalRepo{d} }

// Settings returns the settings store.
func (d *DB) Settings() SettingsRepo { return SettingsRepo{d} }

// The assertions below are the reason a missing method fails at build time rather
// than at the first click in the running app.
var (
	_ task.Repository           = TaskRepo{}
	_ focus.Repository          = SessionRepo{}
	_ ledger.AccountRepository  = AccountRepo{}
	_ ledger.CategoryRepository = CategoryRepo{}
	_ ledger.EntryRepository    = EntryRepo{}
	_ ledger.BudgetRepository   = BudgetRepo{}
	_ template.Repository       = TemplateRepo{}
	_ goal.Repository           = GoalRepo{}
	_ settingsapp.Store         = SettingsRepo{}
)
