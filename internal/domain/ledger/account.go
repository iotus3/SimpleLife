// Package ledger holds money: accounts, categories, entries and monthly budgets. Amounts are
// integer minor units, and CNY and USD are never converted or merged.
package ledger

import (
	"strings"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const (
	// NameMaxRunes bounds an account or category name.
	NameMaxRunes = 40
	// NoteMaxRunes bounds an entry note.
	NoteMaxRunes = 500
	// MaxTags bounds tags per entry.
	MaxTags = 10
	// TagMaxRunes bounds one tag.
	TagMaxRunes = 24
)

// AccountKind classifies an account for grouping and icons.
type AccountKind uint8

const (
	AccountCash AccountKind = iota + 1
	AccountDebitCard
	AccountWallet
	AccountSavings
	AccountCredit
)

// String returns the stable storage key.
func (k AccountKind) String() string {
	switch k {
	case AccountCash:
		return "cash"
	case AccountDebitCard:
		return "debit_card"
	case AccountWallet:
		return "wallet"
	case AccountSavings:
		return "savings"
	case AccountCredit:
		return "credit"
	}
	return "unknown"
}

// ParseAccountKind is the inverse of String.
func ParseAccountKind(s string) (AccountKind, error) {
	switch s {
	case "cash":
		return AccountCash, nil
	case "debit_card":
		return AccountDebitCard, nil
	case "wallet":
		return AccountWallet, nil
	case "savings":
		return AccountSavings, nil
	case "credit":
		return AccountCredit, nil
	}
	return 0, shared.Invalid("ledger.account_kind.parse", "kind", "unknown account kind: "+s)
}

// Account is a wallet, card, or the focus savings pot. BuiltinKey marks a seeded row whose
// display name follows the interface language.
type Account struct {
	ID         shared.ID
	Name       string
	BuiltinKey string
	Kind       AccountKind
	// Initial is the opening balance. Currency is fixed at creation: changing it
	// later would silently reinterpret every historical entry.
	Initial   shared.Money
	Archived  bool
	SortOrder int
}

// NewAccount builds a user account.
func NewAccount(id shared.ID, name string, kind AccountKind, initial shared.Money) (*Account, error) {
	a := &Account{ID: id, Name: name, Kind: kind, Initial: initial}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

// Validate checks the account invariants.
func (a *Account) Validate() error {
	if a.ID.IsZero() {
		return shared.Invalid("ledger.account.id", "", "account id is required")
	}
	name := strings.TrimSpace(a.Name)
	if n := utf8.RuneCountInString(name); n == 0 || n > NameMaxRunes {
		return shared.Invalid("ledger.account.name", "name", "name must be 1..40 runes")
	}
	switch a.Kind {
	case AccountCash, AccountDebitCard, AccountWallet, AccountSavings, AccountCredit:
	default:
		return shared.Invalid("ledger.account.kind", "kind", "unknown account kind")
	}
	if !a.Initial.Currency().Valid() {
		return shared.Invalid("ledger.account.currency", "currency", "currency must be CNY or USD")
	}
	return nil
}

// Rename changes a user account's name. Builtin accounts keep their key and are
// renamed by adding a new account instead, so switching language never leaves a
// half-translated name behind.
func (a *Account) Rename(name string) error {
	if a.BuiltinKey != "" {
		return shared.Conflict("ledger.account.builtin", "a builtin account cannot be renamed; create a new one")
	}
	a.Name = name
	return a.Validate()
}

// Archive hides an account from pickers without deleting its history.
func (a *Account) Archive() error {
	if a.BuiltinKey != "" {
		return shared.Conflict("ledger.account.builtin", "a builtin account cannot be archived")
	}
	a.Archived = true
	return nil
}

// Restore brings an archived account back.
func (a *Account) Restore() { a.Archived = false }

// Currency is the account's currency, the one its amounts are booked in.
func (a *Account) Currency() shared.Currency { return a.Initial.Currency() }

// CategoryDirection is which side of the ledger a category belongs to.
type CategoryDirection uint8

const (
	CategoryExpense CategoryDirection = iota + 1
	CategoryIncome
)

// String returns the stable storage key.
func (d CategoryDirection) String() string {
	switch d {
	case CategoryExpense:
		return "expense"
	case CategoryIncome:
		return "income"
	}
	return "unknown"
}

// ParseCategoryDirection is the inverse of String.
func ParseCategoryDirection(s string) (CategoryDirection, error) {
	switch s {
	case "expense":
		return CategoryExpense, nil
	case "income":
		return CategoryIncome, nil
	}
	return 0, shared.Invalid("ledger.category_direction.parse", "direction", "unknown direction: "+s)
}

// Category groups entries. Two levels at most: a child of a child is refused, so
// the report tree can be rendered in one pass.
type Category struct {
	ID         shared.ID
	Name       string
	BuiltinKey string
	Direction  CategoryDirection
	ParentID   shared.ID
	SortOrder  int
}

// NewCategory builds a user category.
func NewCategory(id shared.ID, name string, dir CategoryDirection) (*Category, error) {
	c := &Category{ID: id, Name: name, Direction: dir}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate checks the category invariants.
func (c *Category) Validate() error {
	if c.ID.IsZero() {
		return shared.Invalid("ledger.category.id", "", "category id is required")
	}
	name := strings.TrimSpace(c.Name)
	if n := utf8.RuneCountInString(name); n == 0 || n > NameMaxRunes {
		return shared.Invalid("ledger.category.name", "name", "name must be 1..40 runes")
	}
	if c.Direction != CategoryExpense && c.Direction != CategoryIncome {
		return shared.Invalid("ledger.category.direction", "direction", "direction must be expense or income")
	}
	if c.ParentID == c.ID {
		return shared.Invalid("ledger.category.parent", "parent_id", "a category cannot be its own parent")
	}
	return nil
}

// AttachParent nests the category under parent, refusing a third level.
func (c *Category) AttachParent(parent *Category) error {
	if parent == nil {
		c.ParentID = ""
		return nil
	}
	if parent.ParentID != "" {
		return shared.Invalid("ledger.category.depth", "parent_id", "categories are two levels deep")
	}
	if parent.Direction != c.Direction {
		return shared.Invalid("ledger.category.direction_mismatch", "parent_id", "parent direction differs")
	}
	c.ParentID = parent.ID
	return nil
}

// Rename changes a user category's name; builtin categories are managed by the
// translation catalogue instead (§5.13.4).
func (c *Category) Rename(name string) error {
	if c.BuiltinKey != "" {
		return shared.Conflict("ledger.category.builtin", "a builtin category cannot be renamed; create a new one")
	}
	c.Name = name
	return c.Validate()
}

// Builtin keys seeded on first run. They are also i18n message ids, which is why
// they live in the domain as constants rather than as free strings.
const (
	BuiltinAccountCash      = "builtin.account.cash"
	BuiltinAccountDebitCard = "builtin.account.debit_card"
	BuiltinAccountSavings   = "builtin.account.savings"
	BuiltinAccountFocusPot  = "builtin.account.focus_savings"

	BuiltinCategoryDining     = "builtin.category.dining"
	BuiltinCategoryTransport  = "builtin.category.transport"
	BuiltinCategoryShopping   = "builtin.category.shopping"
	BuiltinCategoryHousing    = "builtin.category.housing"
	BuiltinCategorySalary     = "builtin.category.salary"
	BuiltinCategoryFocusAward = "builtin.category.focus_reward"
	BuiltinCategoryFocusFine  = "builtin.category.focus_penalty"
)

// AllBuiltinKeys lists every seeded key. The i18n tests iterate this to prove both
// language catalogues contain a translation for each one.
var AllBuiltinKeys = []string{
	BuiltinAccountCash,
	BuiltinAccountDebitCard,
	BuiltinAccountSavings,
	BuiltinAccountFocusPot,
	BuiltinCategoryDining,
	BuiltinCategoryTransport,
	BuiltinCategoryShopping,
	BuiltinCategoryHousing,
	BuiltinCategorySalary,
	BuiltinCategoryFocusAward,
	BuiltinCategoryFocusFine,
}

// AccountRepository is the persistence port for accounts.
type AccountRepository interface {
	Create(a *Account) error
	Update(a *Account) error
	Delete(id shared.ID) error
	List(includeArchived bool) ([]*Account, error)
	Get(id shared.ID) (*Account, error)
	CountEntries(id shared.ID) (int, error)
}

// CategoryRepository is the persistence port for categories.
type CategoryRepository interface {
	Create(c *Category) error
	Update(c *Category) error
	Delete(id shared.ID) error
	List() ([]*Category, error)
	Get(id shared.ID) (*Category, error)
	// CountEntries is the delete guard: a referenced category cannot vanish.
	CountEntries(id shared.ID) (int, error)
	CountChildren(id shared.ID) (int, error)
}
