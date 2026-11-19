package store

import (
	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// Compile-time proof that the in-memory store really implements every port. If a
// port gains a method, this file fails to build — which is the reminder to teach
// the test double about it too.
var (
	_ task.Repository           = TaskRepo{}
	_ focus.Repository          = SessionRepo{}
	_ ledger.AccountRepository  = AccountRepo{}
	_ ledger.CategoryRepository = CategoryRepo{}
	_ ledger.EntryRepository    = EntryRepo{}
	_ ledger.BudgetRepository   = BudgetRepo{}
	_ template.Repository       = TemplateRepo{}
	_ settings.Store            = SettingsRepo{}
	_ shared.TxRunner           = (*Store)(nil)
)
