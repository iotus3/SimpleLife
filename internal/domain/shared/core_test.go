package shared_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// ── errors ─────────────────────────────────────────────────────────────

func TestKindString(t *testing.T) {
	cases := []struct {
		kind shared.Kind
		want string
	}{
		{shared.KindInvalid, "invalid"},
		{shared.KindNotFound, "notfound"},
		{shared.KindConflict, "conflict"},
		{shared.KindUnsupported, "unsupported"},
		{shared.Kind(0), "unknown"},
		{shared.Kind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.want {
			t.Errorf("Kind(%d).String() = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestErrorStringIncludesCodeFieldAndCause(t *testing.T) {
	base := shared.Invalid("task.title.length", "title", "too short")
	if got, want := base.Error(), "task.title.length: too short (field=title)"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	wrapped := base.Wrap(errors.New("root cause"))
	if got, want := wrapped.Error(), "task.title.length: too short (field=title): root cause"; got != want {
		t.Fatalf("wrapped Error() = %q, want %q", got, want)
	}
	plain := shared.NotFound("task.missing", "no such task")
	if got, want := plain.Error(), "task.missing: no such task"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrorConstructorsCarryKind(t *testing.T) {
	cases := []struct {
		err  error
		kind shared.Kind
		code string
	}{
		{shared.Invalid("c.invalid", "f", "m"), shared.KindInvalid, "c.invalid"},
		{shared.NotFound("c.notfound", "m"), shared.KindNotFound, "c.notfound"},
		{shared.Conflict("c.conflict", "m"), shared.KindConflict, "c.conflict"},
		{shared.Unsupported("c.unsupported", "m"), shared.KindUnsupported, "c.unsupported"},
	}
	for _, c := range cases {
		if !shared.IsKind(c.err, c.kind) {
			t.Errorf("%v should be kind %v", c.err, c.kind)
		}
		if got := shared.Code(c.err); got != c.code {
			t.Errorf("Code(%v) = %q, want %q", c.err, got, c.code)
		}
	}
}

func TestIsKindAndCodeOnPlainError(t *testing.T) {
	err := errors.New("plain")
	if shared.IsKind(err, shared.KindInvalid) {
		t.Fatal("plain error must not report a domain kind")
	}
	if got := shared.Code(err); got != "" {
		t.Fatalf("Code(plain) = %q, want empty", got)
	}
	if got := shared.Field(err); got != "" {
		t.Fatalf("Field(plain) = %q, want empty", got)
	}
	if shared.IsKind(nil, shared.KindInvalid) {
		t.Fatal("nil must not report a domain kind")
	}
}

func TestWithFieldDoesNotMutateOriginal(t *testing.T) {
	orig := shared.Invalid("money.format", "", "bad amount")
	withField := orig.WithField("amount")
	if orig.Field != "" {
		t.Fatalf("original Field mutated: %q", orig.Field)
	}
	if withField.Field != "amount" {
		t.Fatalf("copy Field = %q, want amount", withField.Field)
	}
	if withField.Code != orig.Code || withField.Kind != orig.Kind {
		t.Fatal("WithField must keep kind and code")
	}
}

func TestWrapKeepsKindAndUnwrapsToCause(t *testing.T) {
	cause := errors.New("disk full")
	err := shared.Unsupported("export.write", "cannot write").Wrap(cause)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped domain error must unwrap to its cause")
	}
	if !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatal("wrapping must keep the kind")
	}
}

func TestDupfWrapsErrDuplicate(t *testing.T) {
	err := shared.Dupf("entry %s", "abc")
	if !errors.Is(err, shared.ErrDuplicate) {
		t.Fatal("Dupf must wrap ErrDuplicate")
	}
	if got, want := err.Error(), "shared: duplicate key: entry abc"; got != want {
		t.Fatalf("Dupf() = %q, want %q", got, want)
	}
}

// ── clock ──────────────────────────────────────────────────────────────

func TestClockHelpersUseTheInjectedClock(t *testing.T) {
	// 00:30 on the 1st: a UTC-based implementation would report the previous
	// day, which is exactly the bug this helper exists to prevent.
	at := time.Date(2026, 3, 1, 0, 30, 0, 0, time.Local)
	clk := shared.ClockFunc(func() time.Time { return at })

	if got, want := shared.Today(clk), shared.NewLocalDate(2026, 3, 1); !got.Equal(want) {
		t.Fatalf("Today = %v, want %v", got, want)
	}
	if got, want := shared.ThisMonth(clk), shared.MustParseMonth("2026-03"); got != want {
		t.Fatalf("ThisMonth = %v, want %v", got, want)
	}
	if got := clk.Now(); !got.Equal(at) {
		t.Fatalf("Now = %v, want %v", got, at)
	}
}

// ── dates ──────────────────────────────────────────────────────────────

func TestNewLocalDateNormalisesOverflow(t *testing.T) {
	if got, want := shared.NewLocalDate(2026, time.January, 32), shared.MustParseLocalDate("2026-02-01"); !got.Equal(want) {
		t.Fatalf("Jan 32 = %v, want %v", got, want)
	}
	if got, want := shared.NewLocalDate(2024, time.February, 29), shared.MustParseLocalDate("2024-02-29"); !got.Equal(want) {
		t.Fatalf("leap day = %v, want %v", got, want)
	}
}

func TestDateWeekdayAtAndAsMonth(t *testing.T) {
	d := shared.MustParseLocalDate("2026-11-17") // 星期二
	if got := d.Weekday(); got != time.Tuesday {
		t.Fatalf("Weekday = %v, want Tuesday", got)
	}
	got := d.At(9, 30)
	if got.Hour() != 9 || got.Minute() != 30 || got.Day() != 17 {
		t.Fatalf("At = %v", got)
	}
	if m := d.AsMonth(); m != shared.MustParseMonth("2026-11") {
		t.Fatalf("AsMonth = %v", m)
	}
}

func TestMonthIsZeroAndRangeBounds(t *testing.T) {
	var zero shared.Month
	if !zero.IsZero() {
		t.Fatal("zero month must report IsZero")
	}
	if m := shared.MustParseMonth("2026-11"); m.IsZero() {
		t.Fatal("parsed month must not be zero")
	}

	unbounded := shared.DateRange{}
	if !unbounded.IsZero() {
		t.Fatal("zero range must report IsZero")
	}
	if !unbounded.Start().IsZero() || !unbounded.End().IsZero() {
		t.Fatal("unbounded range must have zero instants")
	}
	if !unbounded.Contains(shared.MustParseLocalDate("1999-01-01")) {
		t.Fatal("unbounded range must contain any date")
	}

	m := shared.MustParseMonth("2026-11")
	r := shared.OfMonth(m)
	if !r.Start().Equal(m.Start()) {
		t.Fatal("range start must be the first instant of the month")
	}
	if r.End().Day() != 1 || r.End().Month() != time.December {
		t.Fatalf("range end must be the exclusive first day of the next month, got %v", r.End())
	}
}

func TestDateRangeStartEndWithBounds(t *testing.T) {
	from := shared.MustParseLocalDate("2026-11-01")
	to := shared.MustParseLocalDate("2026-11-08")
	r := shared.DateRange{From: from, To: to}
	if !r.Start().Equal(from.Midnight()) {
		t.Fatal("bounded range start mismatch")
	}
	if !r.End().Equal(to.Midnight()) {
		t.Fatal("bounded range end mismatch")
	}
	if r.IsZero() {
		t.Fatal("bounded range must not be zero")
	}
}

func TestZeroDateStringIsEmpty(t *testing.T) {
	var d shared.LocalDate
	if got := d.String(); got != "" {
		t.Fatalf("zero date String = %q, want empty", got)
	}
}

// ── money ──────────────────────────────────────────────────────────────

func TestMoneySignHelpers(t *testing.T) {
	neg := shared.NewMoney(-1250, shared.CNY)
	if neg.IsZero() || !neg.IsNegative() {
		t.Fatalf("neg flags wrong: zero=%v negative=%v", neg.IsZero(), neg.IsNegative())
	}
	if got := neg.Abs().Minor(); got != 1250 {
		t.Fatalf("Abs = %d, want 1250", got)
	}
	pos := neg.Abs()
	if got := pos.Abs().Minor(); got != 1250 {
		t.Fatalf("Abs of positive must be identity, got %d", got)
	}
	if got := pos.Neg().Minor(); got != -1250 {
		t.Fatalf("Neg = %d, want -1250", got)
	}
	if got := shared.NewMoney(-7, shared.USD).Neg().Minor(); got != 7 {
		t.Fatalf("Neg of negative = %d, want 7", got)
	}
	if !shared.ZeroMoney(shared.USD).IsZero() {
		t.Fatal("ZeroMoney must be zero")
	}
}

func TestMoneySameCurrency(t *testing.T) {
	if !shared.NewMoney(1, shared.CNY).SameCurrency(shared.NewMoney(2, shared.CNY)) {
		t.Fatal("same currency must report true")
	}
	if shared.NewMoney(1, shared.CNY).SameCurrency(shared.NewMoney(2, shared.USD)) {
		t.Fatal("different currency must report false")
	}
}

func TestCurrencySymbolAndDecimals(t *testing.T) {
	if got := shared.CNY.Symbol(); got != "¥" {
		t.Fatalf("CNY symbol = %q", got)
	}
	if got := shared.USD.Symbol(); got != "$" {
		t.Fatalf("USD symbol = %q", got)
	}
	if got := shared.Currency("EUR").Symbol(); got != "" {
		t.Fatalf("unknown currency symbol = %q, want empty", got)
	}
	if got := shared.CNY.Decimals(); got != 2 {
		t.Fatalf("CNY decimals = %d", got)
	}
	if got := shared.Currency("EUR").Decimals(); got != 2 {
		t.Fatalf("unknown currency decimals = %d, want 2", got)
	}
}

func TestMustParseMoneyPanicsOnGarbage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustParseMoney must panic on invalid input")
		}
	}()
	shared.MustParseMoney("abc", shared.CNY)
}

func TestSumMoneyMixesCurrencies(t *testing.T) {
	sum, err := shared.SumMoney(shared.CNY, shared.NewMoney(100, shared.CNY), shared.NewMoney(50, shared.CNY))
	if err != nil || sum.Minor() != 150 {
		t.Fatalf("SumMoney = %v, %v", sum, err)
	}
	if _, err := shared.SumMoney(shared.CNY, shared.NewMoney(1, shared.USD)); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Fatalf("mixing currencies must fail, got %v", err)
	}
	empty, err := shared.SumMoney(shared.USD)
	if err != nil || empty.Minor() != 0 || empty.Currency() != shared.USD {
		t.Fatalf("SumMoney() = %v, %v", empty, err)
	}
}

func TestTallyKeepsFirstSeenOrderAndEmptiness(t *testing.T) {
	var tl shared.Tally
	if !tl.IsEmpty() {
		t.Fatal("fresh tally must be empty")
	}
	if got := tl.Totals(); len(got) != 0 {
		t.Fatalf("fresh tally Totals = %v", got)
	}
	tl.Add(shared.NewMoney(1000, shared.USD)) // USD first on purpose
	tl.Add(shared.NewMoney(500, shared.CNY))
	tl.Add(shared.NewMoney(250, shared.USD))
	if tl.IsEmpty() {
		t.Fatal("tally with amounts must not be empty")
	}
	totals := tl.Totals()
	if len(totals) != 2 {
		t.Fatalf("want 2 subtotals, got %d", len(totals))
	}
	if totals[0].Currency != shared.USD || totals[0].Amount.Minor() != 1250 {
		t.Fatalf("USD subtotal = %+v", totals[0])
	}
	if totals[1].Currency != shared.CNY || totals[1].Amount.Minor() != 500 {
		t.Fatalf("CNY subtotal = %+v", totals[1])
	}
}

// ── ids ────────────────────────────────────────────────────────────────

func TestIDStringAndIsZero(t *testing.T) {
	var zero shared.ID
	if !zero.IsZero() {
		t.Fatal("empty id must be zero")
	}
	if got := zero.String(); got != "" {
		t.Fatalf("zero String = %q", got)
	}
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)
	id, err := shared.NewID(now)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if id.IsZero() {
		t.Fatal("minted id must not be zero")
	}
	if got := id.String(); got != string(id) {
		t.Fatalf("String = %q, want %q", got, id)
	}
}

func TestCrockfordIndexRejectsExcludedLetters(t *testing.T) {
	// I, L, O and U are excluded by Crockford base32 to avoid confusion.
	for _, r := range []byte{'I', 'L', 'O', 'U'} {
		id := shared.ID("0123456789012345678901234" + string(r))
		if id.Valid() {
			t.Fatalf("id ending in %q must be invalid", string(r))
		}
	}
	for _, r := range []byte{'0', '9', 'A', 'H', 'J', 'K', 'M', 'N', 'P', 'T', 'V', 'Z'} {
		id := shared.ID("0123456789012345678901234" + string(r))
		if !id.Valid() {
			t.Fatalf("id ending in %q must be valid", string(r))
		}
	}
	// Lowercase is never produced, so it is not accepted either.
	if shared.ID("0123456789012345678901234a").Valid() {
		t.Fatal("lowercase must be rejected")
	}
}

// ── capabilities & transactions ────────────────────────────────────────

func TestNoopNotifierAndSleepGuard(t *testing.T) {
	var n shared.Notifier = shared.NoopNotifier{}
	if err := n.Notify(shared.Notification{Title: "t", Body: "b", Tag: "tag"}); err != nil {
		t.Fatalf("NoopNotifier.Notify = %v", err)
	}
	if got := n.Capability(); got != shared.NotifyUnsupported {
		t.Fatalf("NoopNotifier.Capability = %v, want NotifyUnsupported", got)
	}

	var g shared.SleepGuard = shared.NoopSleepGuard{}
	release, err := g.Hold("focus")
	if err != nil {
		t.Fatalf("NoopSleepGuard.Hold = %v", err)
	}
	if release == nil {
		t.Fatal("Hold must return a release function")
	}
	release()
	release() // idempotent-safe
}

func TestDirectTxRunnerRunsFnAndPropagatesError(t *testing.T) {
	var runner shared.TxRunner = shared.DirectTxRunner{}
	ran := false
	if err := runner.WithinTx(func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("WithinTx = %v", err)
	}
	if !ran {
		t.Fatal("WithinTx must run fn")
	}

	boom := errors.New("boom")
	if err := runner.WithinTx(func() error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("WithinTx must return fn's error, got %v", err)
	}
}
