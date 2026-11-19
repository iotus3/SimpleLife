// Package ports exposes every persistence port as one value, so a test can run the same
// scenario against the in-memory store and against SQLite.
package ports

import (
	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

// Set is every port the application layer needs, plus the transaction runner.
type Set interface {
	Tasks() task.Repository
	Sessions() focus.Repository
	Accounts() ledger.AccountRepository
	Categories() ledger.CategoryRepository
	Entries() ledger.EntryRepository
	Budgets() ledger.BudgetRepository
	Templates() template.Repository
	Settings() settingsapp.Store

	shared.TxRunner

	// Close releases the resources the set owns. The in-memory store has none.
	Close() error
}

// Memory adapts the in-memory store.
func Memory() Set { return memorySet{store.New()} }

// SQLite adapts a migrated SQLite database.
func SQLite(db *sqlite.DB) Set { return sqliteSet{db} }

type memorySet struct{ *store.Store }

func (s memorySet) Tasks() task.Repository                { return s.Store.Tasks() }
func (s memorySet) Sessions() focus.Repository            { return s.Store.Sessions() }
func (s memorySet) Accounts() ledger.AccountRepository    { return s.Store.Accounts() }
func (s memorySet) Categories() ledger.CategoryRepository { return s.Store.Categories() }
func (s memorySet) Entries() ledger.EntryRepository       { return s.Store.Entries() }
func (s memorySet) Budgets() ledger.BudgetRepository      { return s.Store.Budgets() }
func (s memorySet) Templates() template.Repository        { return s.Store.Templates() }
func (s memorySet) Settings() settingsapp.Store           { return s.Store.Settings() }
func (s memorySet) Close() error                          { return nil }

type sqliteSet struct{ *sqlite.DB }

func (s sqliteSet) Tasks() task.Repository                { return s.DB.Tasks() }
func (s sqliteSet) Sessions() focus.Repository            { return s.DB.Sessions() }
func (s sqliteSet) Accounts() ledger.AccountRepository    { return s.DB.Accounts() }
func (s sqliteSet) Categories() ledger.CategoryRepository { return s.DB.Categories() }
func (s sqliteSet) Entries() ledger.EntryRepository       { return s.DB.Entries() }
func (s sqliteSet) Budgets() ledger.BudgetRepository      { return s.DB.Budgets() }
func (s sqliteSet) Templates() template.Repository        { return s.DB.Templates() }
func (s sqliteSet) Settings() settingsapp.Store           { return s.DB.Settings() }
func (s sqliteSet) Close() error                          { return s.DB.Close() }
