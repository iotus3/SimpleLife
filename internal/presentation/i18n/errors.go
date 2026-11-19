package i18n

import (
	"errors"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Error messages. The domain layer reports a Code and never a sentence; the
// map below is the only place a code becomes something a user can read.
const (
	MsgErrorGeneric MessageID = "error.generic"

	MsgErrTaskTitle    MessageID = "error.task.title"
	MsgErrTaskNotes    MessageID = "error.task.notes"
	MsgErrTaskPriority MessageID = "error.task.priority"
	MsgErrTaskDue      MessageID = "error.task.due"
	MsgErrTaskEstimate MessageID = "error.task.estimate"
	MsgErrTaskTags     MessageID = "error.task.tags"
	MsgErrTaskState    MessageID = "error.task.state"
	MsgErrTaskMissing  MessageID = "error.task.missing"
	MsgErrTaskParent   MessageID = "error.task.parent"

	MsgErrFocusActive   MessageID = "error.focus.active"
	MsgErrFocusState    MessageID = "error.focus.state"
	MsgErrFocusSettings MessageID = "error.focus.settings"
	MsgErrFocusEconomy  MessageID = "error.focus.economy"

	MsgErrLedgerAmount    MessageID = "error.ledger.amount"
	MsgErrLedgerAccount   MessageID = "error.ledger.account"
	MsgErrLedgerAccountIn MessageID = "error.ledger.account_in_use"
	MsgErrLedgerCategory  MessageID = "error.ledger.category"
	MsgErrLedgerCatIn     MessageID = "error.ledger.category_in_use"
	MsgErrLedgerBuiltin   MessageID = "error.ledger.builtin"
	MsgErrLedgerEntry     MessageID = "error.ledger.entry"
	MsgErrLedgerCurrency  MessageID = "error.ledger.currency"
	MsgErrLedgerBudget    MessageID = "error.ledger.budget"
	MsgErrLedgerLocked    MessageID = "error.ledger.locked"

	MsgErrGoalFields MessageID = "error.goal.fields"
	MsgErrGoalDate   MessageID = "error.goal.date"

	MsgErrTemplateRule    MessageID = "error.template.rule"
	MsgErrTemplateFields  MessageID = "error.template.fields"
	MsgErrTemplateMissing MessageID = "error.template.missing"

	MsgErrUpdate      MessageID = "error.update"
	MsgErrStorage     MessageID = "error.storage"
	MsgErrExportPlan  MessageID = "error.export.plan"
	MsgErrExportWrite MessageID = "error.export.write"
	MsgErrSettings    MessageID = "error.settings"
	MsgErrMoney       MessageID = "error.money"
	MsgErrIdentifier  MessageID = "error.identifier"
	MsgErrUnknown     MessageID = "error.unknown_value"
	MsgErrNumber      MessageID = "error.number"
	MsgErrNotFound    MessageID = "error.not_found"
	MsgErrDuplicate   MessageID = "error.duplicate"
)

// codeMessages maps every domain error code to the sentence shown for it; a test fails when a
// code built in the domain or application layer is missing here.
var codeMessages = map[string]MessageID{
	"task.title.length":         MsgErrTaskTitle,
	"task.notes.length":         MsgErrTaskNotes,
	"task.priority.range":       MsgErrTaskPriority,
	"task.estimate.range":       MsgErrTaskEstimate,
	"task.done.range":           MsgErrTaskEstimate,
	"task.focus_minutes.range":  MsgErrTaskEstimate,
	"task.due.range":            MsgErrTaskDue,
	"task.tags.count":           MsgErrTaskTags,
	"task.tag.length":           MsgErrTaskTags,
	"task.status.parse":         MsgErrUnknown,
	"task.id.empty":             MsgErrIdentifier,
	"task.sortkey.empty":        MsgErrIdentifier,
	"task.rank.alphabet":        MsgErrIdentifier,
	"task.rank.lowest":          MsgErrIdentifier,
	"task.rank.order":           MsgErrIdentifier,
	"task.archived":             MsgErrTaskState,
	"task.record.not_open":      MsgErrTaskState,
	"task.reopen.not_done":      MsgErrTaskState,
	"task.restore.not_archived": MsgErrTaskState,
	"task.start.done":           MsgErrTaskState,
	"task.origin.empty":         MsgErrTaskState,
	"task.parent.archived":      MsgErrTaskParent,
	"task.parent.depth":         MsgErrTaskParent,
	"task.parent.self":          MsgErrTaskParent,

	"focus.start.already_active":      MsgErrFocusActive,
	"focus.pause.not_running":         MsgErrFocusState,
	"focus.resume.not_paused":         MsgErrFocusState,
	"focus.cancel.final":              MsgErrFocusState,
	"focus.complete.final":            MsgErrFocusState,
	"focus.complete.empty":            MsgErrFocusState,
	"focus.no_active_session":         MsgErrFocusState,
	"focus.recover.action":            MsgErrFocusState,
	"focus.planned.range":             MsgErrFocusSettings,
	"focus.settings.focus":            MsgErrFocusSettings,
	"focus.settings.short_break":      MsgErrFocusSettings,
	"focus.settings.long_break":       MsgErrFocusSettings,
	"focus.settings.long_break_every": MsgErrFocusSettings,
	"focus.economy.account":           MsgErrFocusEconomy,
	"focus.economy.cap":               MsgErrFocusEconomy,
	"focus.economy.cap_currency":      MsgErrFocusEconomy,
	"focus.economy.category":          MsgErrFocusEconomy,
	"focus.economy.currency":          MsgErrFocusEconomy,
	"focus.economy.penalty_min":       MsgErrFocusEconomy,
	"focus.economy.penalty_ratio":     MsgErrFocusEconomy,
	"focus.economy.rate":              MsgErrFocusEconomy,
	"focus.id.empty":                  MsgErrIdentifier,
	"focus.kind.parse":                MsgErrUnknown,
	"focus.kind.range":                MsgErrUnknown,
	"focus.phase.parse":               MsgErrUnknown,

	"ledger.entry.amount":                MsgErrLedgerAmount,
	"ledger.entry.account":               MsgErrLedgerAccount,
	"ledger.entry.to_account":            MsgErrLedgerAccount,
	"ledger.entry.same_account":          MsgErrLedgerAccount,
	"ledger.entry.unexpected_to_account": MsgErrLedgerAccount,
	"ledger.entry.transfer_category":     MsgErrLedgerCategory,
	"ledger.entry.category":              MsgErrLedgerCategory,
	"ledger.entry.category_direction":    MsgErrLedgerCategory,
	"ledger.account.id":                  MsgErrLedgerAccount,
	"ledger.account.name":                MsgErrLedgerAccount,
	"ledger.account.kind":                MsgErrLedgerAccount,
	"ledger.account.currency":            MsgErrLedgerCurrency,
	"ledger.account.builtin":             MsgErrLedgerBuiltin,
	"ledger.account.in_use":              MsgErrLedgerAccountIn,
	"ledger.account_kind.parse":          MsgErrUnknown,
	"ledger.category.id":                 MsgErrLedgerCategory,
	"ledger.category.name":               MsgErrLedgerCategory,
	"ledger.category.direction":          MsgErrLedgerCategory,
	"ledger.category.direction_mismatch": MsgErrLedgerCategory,
	"ledger.category.parent":             MsgErrLedgerCategory,
	"ledger.category.depth":              MsgErrLedgerCategory,
	"ledger.category.builtin":            MsgErrLedgerBuiltin,
	"ledger.category.in_use":             MsgErrLedgerCatIn,
	"ledger.category.has_children":       MsgErrLedgerCatIn,
	"ledger.category_direction.parse":    MsgErrUnknown,
	"ledger.entry.kind":                  MsgErrUnknown,
	"ledger.entry_kind.parse":            MsgErrUnknown,
	"ledger.origin.parse":                MsgErrUnknown,
	"ledger.entry.currency":              MsgErrLedgerCurrency,
	"ledger.entry.currency_mismatch":     MsgErrLedgerCurrency,
	"ledger.entry.future":                MsgErrLedgerEntry,
	"ledger.entry.locked":                MsgErrLedgerLocked,
	"ledger.entry.id":                    MsgErrLedgerEntry,
	"ledger.entry.note":                  MsgErrLedgerEntry,
	"ledger.entry.tags":                  MsgErrLedgerEntry,
	"ledger.entry.tag":                   MsgErrLedgerEntry,
	"ledger.entry.focus_session":         MsgErrLedgerEntry,
	"ledger.budget.category":             MsgErrLedgerBudget,
	"ledger.budget.id":                   MsgErrLedgerBudget,
	"ledger.budget.limit":                MsgErrLedgerBudget,
	"ledger.budget.month":                MsgErrLedgerBudget,
	"ledger.budget.currency":             MsgErrLedgerCurrency,

	"goal.name.length": MsgErrGoalFields,
	"goal.note.length": MsgErrGoalFields,
	"goal.date.empty":  MsgErrGoalDate,
	"goal.date.order":  MsgErrGoalDate,
	"goal.date.span":   MsgErrGoalDate,
	"goal.id.empty":    MsgErrIdentifier,

	"template.name.length":        MsgErrTemplateFields,
	"template.task.title":         MsgErrTemplateFields,
	"template.task.priority":      MsgErrTemplateFields,
	"template.task.estimate":      MsgErrTemplateFields,
	"template.task.focus_minutes": MsgErrTemplateFields,
	"template.task.due_offset":    MsgErrTemplateFields,
	"template.rule.kind":          MsgErrTemplateRule,
	"template.rule.weekdays":      MsgErrTemplateRule,
	"template.rule.day_of_month":  MsgErrTemplateRule,
	"template.rule.dates":         MsgErrTemplateRule,
	"template.rule.every_n_days":  MsgErrTemplateRule,
	"template.rule.anchor":        MsgErrTemplateRule,
	"template.expand.day":         MsgErrTemplateRule,
	"template.id.empty":           MsgErrTemplateMissing,

	"export.table.unknown":    MsgErrExportPlan,
	"export.tables.empty":     MsgErrExportPlan,
	"export.tables.duplicate": MsgErrExportPlan,
	"export.tables.multiple":  MsgErrExportPlan,
	"export.column.unknown":   MsgErrExportPlan,
	"export.columns.empty":    MsgErrExportPlan,
	"export.currency":         MsgErrExportPlan,
	"export.layout.parse":     MsgErrExportPlan,
	"export.layout.range":     MsgErrExportPlan,

	"ui.number": MsgErrNumber,

	"settings.currency": MsgErrSettings,
	"settings.language": MsgErrSettings,
	"settings.theme":    MsgErrSettings,
	"settings.close":    MsgErrSettings,

	// A repository reports the aggregate it could not find, so a stale row on
	// screen reads as "that record is gone" rather than as a failure.
	"task.not_found":            MsgErrNotFound,
	"ledger.account.not_found":  MsgErrNotFound,
	"ledger.category.not_found": MsgErrNotFound,
	"ledger.entry.not_found":    MsgErrNotFound,
	"ledger.budget.not_found":   MsgErrNotFound,
	"focus.not_found":           MsgErrNotFound,
	"template.not_found":        MsgErrNotFound,
	"goal.not_found":            MsgErrNotFound,
	"template.log.at":           MsgErrTemplateFields,

	// The database itself: a path that cannot be used, a file that fails its own
	// integrity check, a foreign key the file does not honour.
	"sqlite.path.empty":  MsgErrStorage,
	"sqlite.check":       MsgErrStorage,
	"sqlite.foreign_key": MsgErrStorage,

	// Importing a file: one sentence covers the structural failures ("this file is
	// not a table of mine"), because the row reports name the real cause.
	"ui.import.file_type":    MsgDataImportUnknownFile,
	"import.file.empty":      MsgErrImport,
	"import.file.layout":     MsgErrImport,
	"import.file.csv":        MsgErrImport,
	"import.file.markdown":   MsgErrImport,
	"import.file.xlsx":       MsgErrImport,
	"import.header":          MsgErrImport,
	"import.table":           MsgErrImport,
	"import.id":              MsgErrImport,
	"import.kind":            MsgErrImport,
	"import.month":           MsgErrImport,
	"import.number":          MsgErrImport,
	"import.number.range":    MsgErrImport,
	"import.date":            MsgErrImport,
	"import.time":            MsgErrImport,
	"import.currency":        MsgErrImport,
	"import.amount":          MsgErrImport,
	"import.entry.account":   MsgErrImport,
	"import.entry.category":  MsgErrImport,
	"import.category.parent": MsgErrImport,
	"template.rule.parse":    MsgErrTemplateRule,

	"update.not_configured":   MsgErrUpdate,
	"update.no_url":           MsgErrUpdate,
	"update.manifest":         MsgErrUpdate,
	"update.manifest.version": MsgErrUpdate,
	"update.url":              MsgErrUpdate,
	"update.read":             MsgErrUpdate,
	"update.status":           MsgErrUpdate,
	"update.unreachable":      MsgErrUpdate,

	"money.format":           MsgErrMoney,
	"money.scale":            MsgErrMoney,
	"money.empty":            MsgErrMoney,
	"money.range":            MsgErrMoney,
	"money.currency.unknown": MsgErrMoney,
	"date.empty":             MsgErrLedgerEntry,
	"date.format":            MsgErrLedgerEntry,
	"month.format":           MsgErrLedgerEntry,
}

// MessageFor turns an error into a sentence for the user. Repository-level
// sentinels without a code are recognized by identity; everything else falls
// back to the generic failure.
func MessageFor(l Lang, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, shared.ErrNotFound):
		return T(l, MsgErrNotFound)
	case errors.Is(err, shared.ErrDuplicate):
		return T(l, MsgErrDuplicate)
	}
	return MessageForCode(l, shared.Code(err), shared.Field(err))
}

// MessageForCode turns a domain code into a sentence. field is accepted so a
// caller can highlight the offending input; the catalogs name the field in the
// sentence already, so it is not repeated here.
func MessageForCode(l Lang, code, field string) string {
	_ = field
	if id, ok := codeMessages[code]; ok {
		return T(l, id)
	}
	return T(l, MsgErrorGeneric)
}

// CodeMessage returns the message id a code maps to, for tests and tooling.
func CodeMessage(code string) (MessageID, bool) {
	id, ok := codeMessages[code]
	return id, ok
}
