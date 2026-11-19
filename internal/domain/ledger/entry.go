package ledger

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// EntryKind is the direction of a ledger line.
type EntryKind uint8

const (
	EntryExpense EntryKind = iota + 1
	EntryIncome
	EntryTransfer
)

// String returns the stable storage key, also used verbatim in exports.
func (k EntryKind) String() string {
	switch k {
	case EntryExpense:
		return "expense"
	case EntryIncome:
		return "income"
	case EntryTransfer:
		return "transfer"
	}
	return "unknown"
}

// ParseEntryKind is the inverse of String.
func ParseEntryKind(s string) (EntryKind, error) {
	switch s {
	case "expense":
		return EntryExpense, nil
	case "income":
		return EntryIncome, nil
	case "transfer":
		return EntryTransfer, nil
	}
	return 0, shared.Invalid("ledger.entry_kind.parse", "kind", "unknown entry kind: "+s)
}

// Origin says who produced the line: the user, or the pomodoro economy.
type Origin uint8

const (
	OriginManual Origin = iota + 1
	OriginFocusReward
)

// String returns the stable storage key.
func (o Origin) String() string {
	switch o {
	case OriginManual:
		return "manual"
	case OriginFocusReward:
		return "focus_reward"
	}
	return "unknown"
}

// ParseOrigin is the inverse of String.
func ParseOrigin(s string) (Origin, error) {
	switch s {
	case "manual":
		return OriginManual, nil
	case "focus_reward":
		return OriginFocusReward, nil
	}
	return 0, shared.Invalid("ledger.origin.parse", "origin", "unknown origin: "+s)
}

// Entry is one ledger line. Amount is always positive; the sign lives in Kind.
type Entry struct {
	ID             shared.ID
	Kind           EntryKind
	Amount         shared.Money
	AccountID      shared.ID // the paying/receiving account; the source of a transfer
	ToAccountID    shared.ID // transfers only
	CategoryID     shared.ID // empty for transfers
	OccurredAt     time.Time
	Note           string
	Tags           []string
	Origin         Origin
	FocusSessionID shared.ID // set when Origin is OriginFocusReward
	CreatedAt      time.Time
}

// EntryInput is the raw form data NewEntry validates.
type EntryInput struct {
	Kind           EntryKind
	Amount         shared.Money
	AccountID      shared.ID
	ToAccountID    shared.ID
	CategoryID     shared.ID
	OccurredAt     time.Time
	Note           string
	Tags           []string
	Origin         Origin
	FocusSessionID shared.ID
}

// EntryFutureTolerance is how far in the future an entry may be dated before it is
// refused. A small window absorbs clock skew and time-zone edges; a typo of 2062
// is still rejected.
const EntryFutureTolerance = 24 * time.Hour

// NewEntry is the only constructor: the amount is positive, a transfer needs a distinct
// destination and no category, an expense or income needs a category, and the date is not
// more than a day ahead.
func NewEntry(id shared.ID, now time.Time, in EntryInput) (*Entry, error) {
	if id.IsZero() {
		return nil, shared.Invalid("ledger.entry.id", "", "entry id is required")
	}
	if !in.Amount.Currency().Valid() {
		return nil, shared.Invalid("ledger.entry.currency", "currency", "currency must be CNY or USD")
	}
	if in.Amount.Minor() <= 0 {
		return nil, shared.Invalid("ledger.entry.amount", "amount", "amount must be greater than zero")
	}
	if in.AccountID.IsZero() {
		return nil, shared.Invalid("ledger.entry.account", "account_id", "account is required")
	}
	switch in.Kind {
	case EntryTransfer:
		if in.ToAccountID.IsZero() {
			return nil, shared.Invalid("ledger.entry.to_account", "to_account_id", "a transfer needs a destination account")
		}
		if in.ToAccountID == in.AccountID {
			return nil, shared.Invalid("ledger.entry.same_account", "to_account_id", "the two accounts must differ")
		}
		if !in.CategoryID.IsZero() {
			return nil, shared.Invalid("ledger.entry.transfer_category", "category_id", "a transfer has no category")
		}
	case EntryExpense, EntryIncome:
		if in.CategoryID.IsZero() {
			return nil, shared.Invalid("ledger.entry.category", "category_id", "category is required")
		}
		if !in.ToAccountID.IsZero() {
			return nil, shared.Invalid("ledger.entry.unexpected_to_account", "to_account_id", "only a transfer has a destination account")
		}
	default:
		return nil, shared.Invalid("ledger.entry.kind", "kind", "unknown entry kind")
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = now
	}
	if in.OccurredAt.After(now.Add(EntryFutureTolerance)) {
		return nil, shared.Invalid("ledger.entry.future", "occurred_at", "date is in the future")
	}
	note := strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(note); n > NoteMaxRunes {
		return nil, shared.Invalid("ledger.entry.note", "note", "note must be at most 500 runes")
	}
	tags, err := normalizeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	origin := in.Origin
	if origin == 0 {
		origin = OriginManual
	}
	if origin == OriginFocusReward && in.FocusSessionID.IsZero() {
		return nil, shared.Invalid("ledger.entry.focus_session", "", "a focus entry needs its session id")
	}
	return &Entry{
		ID:             id,
		Kind:           in.Kind,
		Amount:         in.Amount,
		AccountID:      in.AccountID,
		ToAccountID:    in.ToAccountID,
		CategoryID:     in.CategoryID,
		OccurredAt:     in.OccurredAt,
		Note:           note,
		Tags:           tags,
		Origin:         origin,
		FocusSessionID: in.FocusSessionID,
		CreatedAt:      now,
	}, nil
}

// SignedAmount is this entry's effect on the balance of AccountID. The receiving side of a
// transfer is summed separately by ToAccountID.
func (e *Entry) SignedAmount() int64 {
	switch e.Kind {
	case EntryIncome:
		return e.Amount.Minor()
	case EntryExpense, EntryTransfer:
		return -e.Amount.Minor()
	}
	return 0
}

// EffectOn is the entry's effect on one specific account, covering both sides of a
// transfer.
func (e *Entry) EffectOn(accountID shared.ID) int64 {
	if e.Kind == EntryTransfer && e.ToAccountID == accountID {
		return e.Amount.Minor()
	}
	if e.AccountID != accountID {
		return 0
	}
	return e.SignedAmount()
}

// IsTransferCredit reports whether this entry credits the given account through a
// transfer, which is needed to keep transfers out of income statistics.
func (e *Entry) IsTransferCredit(accountID shared.ID) bool {
	return e.Kind == EntryTransfer && e.ToAccountID == accountID
}

func normalizeTags(tags []string) ([]string, error) {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" || seen[tag] {
			continue
		}
		if n := utf8.RuneCountInString(tag); n > TagMaxRunes {
			return nil, shared.Invalid("ledger.entry.tag", "tags", "tag must be at most 24 runes")
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return nil, shared.Invalid("ledger.entry.tags", "tags", "at most 10 tags")
	}
	return out, nil
}

// EntryFilter is the query object for ledger lines. Zero values mean "unset".
type EntryFilter struct {
	From, To   time.Time // [From, To)
	AccountID  shared.ID
	CategoryID shared.ID
	Kind       []EntryKind
	Origin     []Origin
	Currency   shared.Currency
	Keyword    string
	Limit      int
	Offset     int
	// NewestFirst is the default order for the ledger page.
	NewestFirst bool
}

// AggregateGroup selects what Aggregate groups by.
type AggregateGroup uint8

const (
	GroupNone AggregateGroup = iota
	GroupDay
	GroupMonth
	GroupCategory
	GroupAccount
)

// AggregateRow is one grouped subtotal. Sum is signed: expenses are negative.
type AggregateRow struct {
	Key      string
	Sum      shared.Money
	Count    int
	Currency shared.Currency
}

// AccountBalanceRow is one account's net movement in minor units. Both sides of a
// transfer are included, so summing the rows and adding each account's opening
// balance yields the balance the user sees.
type AccountBalanceRow struct {
	AccountID shared.ID
	Sum       int64
	Currency  shared.Currency
}

// EntryRepository is the persistence port for entries.
type EntryRepository interface {
	Create(e *Entry) error
	Update(e *Entry) error
	Delete(id shared.ID) error
	Get(id shared.ID) (*Entry, error)
	List(f EntryFilter) ([]*Entry, error)
	Count(f EntryFilter) (int, error)
	// Aggregate sums in the store (SQL SUM / in-memory loop), so ten thousand
	// rows never travel into the UI layer just to be added up.
	Aggregate(f EntryFilter, groupBy AggregateGroup) ([]AggregateRow, error)
	// SumByOrigin totals one origin and direction in a period for one currency, which is how the
	// daily cap is enforced without scanning the ledger.
	SumByOrigin(origin Origin, kind EntryKind, from, to time.Time, currency shared.Currency) (shared.Money, error)
	// ExistsFocusSession reports whether a session already produced an entry.
	ExistsFocusSession(sessionID shared.ID) (bool, error)
	// AccountBalances returns each account's net movement over the filtered
	// entries, transfer credits included. It exists so the balance never has to be
	// recomputed by loading every row into the application layer.
	AccountBalances(f EntryFilter) ([]AccountBalanceRow, error)
}
