package shared

import (
	"errors"
	"strconv"
	"strings"
)

// Currency is a supported money kind. The list is closed on purpose: an
// unknown currency would otherwise reach the database and the UI.
type Currency string

const (
	CNY Currency = "CNY"
	USD Currency = "USD"
)

// Valid reports whether c is one of the supported currencies.
func (c Currency) Valid() bool { return c == CNY || c == USD }

// Symbol returns the currency sign for display. Real formatting (grouping,
// decimals, sign placement) lives in presentation/format.
func (c Currency) Symbol() string {
	switch c {
	case CNY:
		return "¥"
	case USD:
		return "$"
	}
	return ""
}

// Decimals is how many digits follow the decimal separator. Both supported
// currencies use 2, but the accessor keeps the model honest if one is added.
func (c Currency) Decimals() int {
	switch c {
	case CNY, USD:
		return 2
	}
	return 2
}

// DefaultCurrency is what a new account and new settings start with.
const DefaultCurrency = CNY

// ErrCurrencyMismatch is returned by any arithmetic across two currencies.
// SimpleLife has no exchange rates (ADR-011): mixing is an error, never a guess.
var ErrCurrencyMismatch = errors.New("shared: currency mismatch")

// Money is an exact amount in the currency's minor unit (分 for CNY, cent for
// USD). Floating point is never used, so 0.10 + 0.20 is exactly 0.30.
type Money struct {
	minor    int64
	currency Currency
}

// NewMoney builds an amount from minor units.
func NewMoney(minor int64, c Currency) Money { return Money{minor: minor, currency: c} }

// ZeroMoney is the zero amount in c.
func ZeroMoney(c Currency) Money { return Money{currency: c} }

// Minor returns the amount in minor units.
func (m Money) Minor() int64 { return m.minor }

// Currency returns the amount's currency.
func (m Money) Currency() Currency { return m.currency }

// IsZero reports a zero amount.
func (m Money) IsZero() bool { return m.minor == 0 }

// IsNegative reports a negative amount.
func (m Money) IsNegative() bool { return m.minor < 0 }

// Abs returns the amount without its sign, keeping the currency.
func (m Money) Abs() Money {
	if m.minor < 0 {
		return Money{minor: -m.minor, currency: m.currency}
	}
	return m
}

// Neg returns the amount with the opposite sign.
func (m Money) Neg() Money { return Money{minor: -m.minor, currency: m.currency} }

// Add adds two amounts of the same currency.
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{minor: m.minor + o.minor, currency: m.currency}, nil
}

// Sub subtracts o from m; both must share the currency.
func (m Money) Sub(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{minor: m.minor - o.minor, currency: m.currency}, nil
}

// Equal reports equal amount and currency.
func (m Money) Equal(o Money) bool { return m.minor == o.minor && m.currency == o.currency }

// SameCurrency reports whether two amounts can be added at all.
func (m Money) SameCurrency(o Money) bool { return m.currency == o.currency }

// String renders a canonical, currency-less form used by storage and exports:
// "-12.34". The UI adds the symbol.
func (m Money) String() string {
	neg := m.minor < 0
	units := m.minor
	if neg {
		units = -units
	}
	d := m.currency.Decimals()
	div := int64(1)
	for i := 0; i < d; i++ {
		div *= 10
	}
	whole := units / div
	frac := units % div
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(strconv.FormatInt(whole, 10))
	if d > 0 {
		b.WriteByte('.')
		fs := strconv.FormatInt(frac, 10)
		for i := len(fs); i < d; i++ {
			b.WriteByte('0')
		}
		b.WriteString(fs)
	}
	return b.String()
}

// ParseMoney parses user input into an exact amount: an optional sign, thousands separators,
// at most Decimals() fraction digits, ignoring spaces and the currency sign. Anything else is
// refused rather than rounded.
func ParseMoney(s string, c Currency) (Money, error) {
	if !c.Valid() {
		return Money{}, Invalid("money.currency.unknown", "", "unsupported currency "+string(c))
	}
	orig := s
	s = strings.TrimSpace(s)
	// Drop currency symbols, spaces and common separators users paste in.
	repl := strings.NewReplacer(
		"¥", "", "$", "", "＄", "", "￥", "", " ", "", "\u00a0", "",
		",", "", "，", "", "_", "",
	)
	s = repl.Replace(s)
	if s == "" {
		return Money{}, Invalid("money.empty", "", "amount is empty")
	}
	neg := false
	switch {
	case strings.HasPrefix(s, "\u2212"): // U+2212 MINUS SIGN, typed by some IMEs
		neg = true
		s = s[len("\u2212"):]
	case s[0] == '-':
		neg = true
		s = s[1:]
	case s[0] == '+':
		s = s[1:]
	}
	if s == "" {
		return Money{}, Invalid("money.empty", "", "amount is empty: "+orig)
	}
	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return Money{}, Invalid("money.format", "", "amount is not a number: "+orig)
		}
	}
	if len(fracPart) > c.Decimals() {
		return Money{}, Invalid("money.scale", "", "amount has more than "+strconv.Itoa(c.Decimals())+" decimals: "+orig)
	}
	for _, r := range fracPart {
		if r < '0' || r > '9' {
			return Money{}, Invalid("money.format", "", "amount is not a number: "+orig)
		}
	}
	whole := int64(0)
	for i := 0; i < len(intPart); i++ {
		whole = whole*10 + int64(intPart[i]-'0')
		if whole > (1<<62)/10 {
			return Money{}, Invalid("money.range", "", "amount is too large: "+orig)
		}
	}
	div := int64(1)
	for i := 0; i < c.Decimals(); i++ {
		div *= 10
	}
	minor := whole * div
	if fracPart != "" {
		f := int64(0)
		for i := 0; i < len(fracPart); i++ {
			f = f*10 + int64(fracPart[i]-'0')
		}
		for i := len(fracPart); i < c.Decimals(); i++ {
			f *= 10
		}
		minor += f
	}
	if neg {
		minor = -minor
	}
	return Money{minor: minor, currency: c}, nil
}

// MustParseMoney is ParseMoney for tests and constants.
func MustParseMoney(s string, c Currency) Money {
	m, err := ParseMoney(s, c)
	if err != nil {
		panic(err)
	}
	return m
}

// SumMoney adds amounts, refusing to mix currencies. An empty slice yields zero
// in c.
func SumMoney(c Currency, amounts ...Money) (Money, error) {
	total := ZeroMoney(c)
	for _, m := range amounts {
		var err error
		total, err = total.Add(m)
		if err != nil {
			return Money{}, err
		}
	}
	return total, nil
}

// MoneyTotal is one currency's subtotal. Every aggregate returns a slice of
// these instead of a single number (ADR-011).
type MoneyTotal struct {
	Currency Currency
	Amount   Money
}

// Tally accumulates amounts by currency, keeping insertion order stable so the
// UI shows CNY before USD regardless of arrival order.
type Tally struct {
	order  []Currency
	totals map[Currency]Money
}

// Add accumulates m.
func (t *Tally) Add(m Money) {
	if t.totals == nil {
		t.totals = make(map[Currency]Money, 2)
	}
	if _, ok := t.totals[m.Currency()]; !ok {
		t.order = append(t.order, m.Currency())
	}
	cur := t.totals[m.Currency()]
	cur = NewMoney(cur.Minor()+m.Minor(), m.Currency())
	t.totals[m.Currency()] = cur
}

// Totals returns the accumulated subtotals in stable order.
func (t *Tally) Totals() []MoneyTotal {
	out := make([]MoneyTotal, 0, len(t.order))
	for _, c := range t.order {
		out = append(out, MoneyTotal{Currency: c, Amount: t.totals[c]})
	}
	return out
}

// IsEmpty reports whether nothing was accumulated.
func (t *Tally) IsEmpty() bool { return len(t.order) == 0 }
