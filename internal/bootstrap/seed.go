package bootstrap

import (
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// accountSpec and categorySpec describe one built-in row: the stable key that is
// stored, and the shape the row has.
type accountSpec struct {
	key  string
	kind ledger.AccountKind
}

type categorySpec struct {
	key       string
	direction ledger.CategoryDirection
}

// builtinAccounts is the seed of a fresh database, in display order.
var builtinAccounts = []accountSpec{
	{key: ledger.BuiltinAccountCash, kind: ledger.AccountCash},
	{key: ledger.BuiltinAccountDebitCard, kind: ledger.AccountDebitCard},
	{key: ledger.BuiltinAccountSavings, kind: ledger.AccountSavings},
	{key: ledger.BuiltinAccountFocusPot, kind: ledger.AccountSavings},
}

// builtinCategories is the seed of a fresh database, in display order.
var builtinCategories = []categorySpec{
	{key: ledger.BuiltinCategoryDining, direction: ledger.CategoryExpense},
	{key: ledger.BuiltinCategoryTransport, direction: ledger.CategoryExpense},
	{key: ledger.BuiltinCategoryShopping, direction: ledger.CategoryExpense},
	{key: ledger.BuiltinCategoryHousing, direction: ledger.CategoryExpense},
	{key: ledger.BuiltinCategorySalary, direction: ledger.CategoryIncome},
	{key: ledger.BuiltinCategoryFocusAward, direction: ledger.CategoryIncome},
	{key: ledger.BuiltinCategoryFocusFine, direction: ledger.CategoryExpense},
}

// newBuiltinAccount builds a seeded account. The name is the translation of the
// key at first-run time and is only a fallback: the interface displays the key.
func newBuiltinAccount(id shared.ID, lang i18n.Lang, spec accountSpec, currency shared.Currency) (*ledger.Account, error) {
	account, err := ledger.NewAccount(id, i18n.T(lang, i18n.MessageID(spec.key)), spec.kind, shared.ZeroMoney(currency))
	if err != nil {
		return nil, err
	}
	account.BuiltinKey = spec.key
	return account, nil
}

// newBuiltinCategory builds a seeded category with the same fallback name.
func newBuiltinCategory(id shared.ID, lang i18n.Lang, spec categorySpec) (*ledger.Category, error) {
	category, err := ledger.NewCategory(id, i18n.T(lang, i18n.MessageID(spec.key)), spec.direction)
	if err != nil {
		return nil, err
	}
	category.BuiltinKey = spec.key
	return category, nil
}
