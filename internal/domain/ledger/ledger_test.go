package ledger_test

import (
	"strings"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func cny(minor int64) shared.Money { return shared.NewMoney(minor, shared.CNY) }

func TestNewEntryValidatesTheBasics(t *testing.T) {
	now := at("2026-11-17 12:00:00")
	base := ledger.EntryInput{
		Kind:       ledger.EntryExpense,
		Amount:     cny(3550),
		AccountID:  "01J0ACCOUNTCASH0000000000A",
		CategoryID: "01J0CATEGORYDINING0000000A",
		OccurredAt: now,
	}
	if _, err := ledger.NewEntry("01J0ENTRY000000000000001", now, base); err != nil {
		t.Fatalf("valid expense rejected: %v", err)
	}

	cases := []struct {
		name  string
		code  string
		mutc  func(*ledger.EntryInput)
		sets  func(*ledger.EntryInput)
		empty bool
	}{
		{name: "空 id", mutc: func(in *ledger.EntryInput) {}, sets: func(in *ledger.EntryInput) {}},
		{name: "金额为零", mutc: func(in *ledger.EntryInput) { in.Amount = cny(0) }},
		{name: "金额为负", mutc: func(in *ledger.EntryInput) { in.Amount = cny(-1) }},
		{name: "币种非法", mutc: func(in *ledger.EntryInput) { in.Amount = shared.NewMoney(100, shared.Currency("EUR")) }},
		{name: "缺账户", mutc: func(in *ledger.EntryInput) { in.AccountID = "" }},
		{name: "缺分类", mutc: func(in *ledger.EntryInput) { in.CategoryID = "" }},
		{name: "未知类型", mutc: func(in *ledger.EntryInput) { in.Kind = ledger.EntryKind(9) }},
		{name: "未来日期", mutc: func(in *ledger.EntryInput) { in.OccurredAt = now.Add(48 * time.Hour) }},
		{name: "备注过长", mutc: func(in *ledger.EntryInput) { in.Note = strings.Repeat("x", ledger.NoteMaxRunes+1) }},
		{name: "标签过长", mutc: func(in *ledger.EntryInput) { in.Tags = []string{strings.Repeat("t", ledger.TagMaxRunes+1)} }},
	}
	for i, c := range cases {
		in := base
		if c.mutc != nil {
			c.mutc(&in)
		}
		id := shared.ID("01J0ENTRY000000000000001")
		if i == 0 {
			id = ""
		}
		if _, err := ledger.NewEntry(id, now, in); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("%s must be invalid, got %v", c.name, err)
		}
	}
}

func TestNewEntryTransferRules(t *testing.T) {
	now := at("2026-11-17 12:00:00")
	transfer := ledger.EntryInput{
		Kind:        ledger.EntryTransfer,
		Amount:      cny(10000),
		AccountID:   "01J0ACCOUNTCASH0000000000A",
		ToAccountID: "01J0ACCOUNTBANK0000000000B",
		OccurredAt:  now,
	}
	if _, err := ledger.NewEntry("01J0ENTRY000000000000002", now, transfer); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}

	noDest := transfer
	noDest.ToAccountID = ""
	if got := shared.Code(mustRejected(t, now, noDest)); got != "ledger.entry.to_account" {
		t.Fatalf("code = %q", got)
	}

	same := transfer
	same.ToAccountID = same.AccountID
	if got := shared.Code(mustRejected(t, now, same)); got != "ledger.entry.same_account" {
		t.Fatalf("code = %q", got)
	}

	withCategory := transfer
	withCategory.CategoryID = "01J0CATEGORYDINING0000000A"
	if got := shared.Code(mustRejected(t, now, withCategory)); got != "ledger.entry.transfer_category" {
		t.Fatalf("code = %q", got)
	}

	expenseWithDest := transfer
	expenseWithDest.Kind = ledger.EntryExpense
	expenseWithDest.CategoryID = "01J0CATEGORYDINING0000000A"
	if got := shared.Code(mustRejected(t, now, expenseWithDest)); got != "ledger.entry.unexpected_to_account" {
		t.Fatalf("code = %q", got)
	}
}

func TestNewEntryFocusOriginNeedsSession(t *testing.T) {
	now := at("2026-11-17 12:00:00")
	in := ledger.EntryInput{
		Kind:       ledger.EntryIncome,
		Amount:     cny(1250),
		AccountID:  "01J0ACCOUNTPOT0000000000A",
		CategoryID: "01J0CATEGORYAWARD0000000A",
		Origin:     ledger.OriginFocusReward,
	}
	if _, err := ledger.NewEntry("01J0ENTRY000000000000003", now, in); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a focus entry without a session must be invalid, got %v", err)
	}
	in.FocusSessionID = "01J0FOCUS000000000000001"
	e, err := ledger.NewEntry("01J0ENTRY000000000000003", now, in)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if e.Origin != ledger.OriginFocusReward || e.FocusSessionID == "" {
		t.Fatalf("origin not recorded: %+v", e)
	}
	zero := in
	zero.Origin = 0
	e, err = ledger.NewEntry("01J0ENTRY000000000000003", now, zero)
	if err != nil {
		t.Fatal(err)
	}
	if e.Origin != ledger.OriginManual {
		t.Fatalf("a missing origin must default to manual, got %v", e.Origin)
	}
	inOccurred := in
	inOccurred.OccurredAt = time.Time{}
	e, err = ledger.NewEntry("01J0ENTRY000000000000003", now, inOccurred)
	if err != nil {
		t.Fatal(err)
	}
	if !e.OccurredAt.Equal(now) {
		t.Fatalf("a missing date must default to now, got %v", e.OccurredAt)
	}
}

func TestEntryNormalisesNoteAndTags(t *testing.T) {
	now := at("2026-11-17 12:00:00")
	e, err := ledger.NewEntry("01J0ENTRY000000000000004", now, ledger.EntryInput{
		Kind:       ledger.EntryExpense,
		Amount:     cny(100),
		AccountID:  "01J0ACCOUNTCASH0000000000A",
		CategoryID: "01J0CATEGORYDINING0000000A",
		Note:       "  午饭  ",
		Tags:       []string{" Food ", "food", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Note != "午饭" {
		t.Fatalf("note = %q", e.Note)
	}
	if len(e.Tags) != 1 || e.Tags[0] != "food" {
		t.Fatalf("tags = %v", e.Tags)
	}
	if !e.CreatedAt.Equal(now) {
		t.Fatal("CreatedAt must come from the injected clock")
	}
}

func TestSignedAmountAndEffect(t *testing.T) {
	now := at("2026-11-17 12:00:00")
	expense := mustEntry(t, now, ledger.EntryInput{
		Kind: ledger.EntryExpense, Amount: cny(3550),
		AccountID: "01J0ACCOUNTCASH0000000000A", CategoryID: "01J0CATEGORYDINING0000000A",
	})
	if got := expense.SignedAmount(); got != -3550 {
		t.Fatalf("expense SignedAmount = %d", got)
	}
	income := mustEntry(t, now, ledger.EntryInput{
		Kind: ledger.EntryIncome, Amount: cny(500000),
		AccountID: "01J0ACCOUNTBANK0000000000B", CategoryID: "01J0CATEGORYSALARY0000000A",
	})
	if got := income.SignedAmount(); got != 500000 {
		t.Fatalf("income SignedAmount = %d", got)
	}
	transfer := mustEntry(t, now, ledger.EntryInput{
		Kind: ledger.EntryTransfer, Amount: cny(20000),
		AccountID: "01J0ACCOUNTBANK0000000000B", ToAccountID: "01J0ACCOUNTCASH0000000000A",
	})
	if got := transfer.SignedAmount(); got != -20000 {
		t.Fatalf("transfer SignedAmount = %d", got)
	}
	if got := transfer.EffectOn("01J0ACCOUNTBANK0000000000B"); got != -20000 {
		t.Fatalf("transfer source effect = %d", got)
	}
	if got := transfer.EffectOn("01J0ACCOUNTCASH0000000000A"); got != 20000 {
		t.Fatalf("transfer destination effect = %d", got)
	}
	if got := transfer.EffectOn("01J0ACCOUNTOTHER000000000C"); got != 0 {
		t.Fatalf("unrelated account effect = %d", got)
	}
	if !transfer.IsTransferCredit("01J0ACCOUNTCASH0000000000A") {
		t.Fatal("the destination side must be a transfer credit")
	}
	if transfer.IsTransferCredit("01J0ACCOUNTBANK0000000000B") || expense.IsTransferCredit("01J0ACCOUNTCASH0000000000A") {
		t.Fatal("IsTransferCredit is wrong")
	}
}

func TestAccountValidateAndGuards(t *testing.T) {
	a, err := ledger.NewAccount("01J0ACCOUNTCASH0000000000A", "  现金  ", ledger.AccountCash, cny(0))
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if a.Name != "  现金  " {
		// NewAccount validates a trimmed copy but keeps the caller's value; the
		// adapter stores the trimmed name.
		if strings.TrimSpace(a.Name) != "现金" {
			t.Fatalf("name = %q", a.Name)
		}
	}
	if err := a.Rename("  零钱  "); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if !a.Archived {
		a.Archive()
	}
	if !a.Archived {
		t.Fatal("Archive must set the flag")
	}
	a.Restore()
	if a.Archived {
		t.Fatal("Restore must clear the flag")
	}
	if a.Currency() != shared.CNY {
		t.Fatalf("currency = %v", a.Currency())
	}

	bad := []*ledger.Account{
		{Name: "x", Kind: ledger.AccountCash, Initial: cny(0)},
		{ID: "01J0ACCOUNTCASH0000000000A", Name: " ", Kind: ledger.AccountCash, Initial: cny(0)},
		{ID: "01J0ACCOUNTCASH0000000000A", Name: strings.Repeat("名", ledger.NameMaxRunes+1), Kind: ledger.AccountCash},
		{ID: "01J0ACCOUNTCASH0000000000A", Name: "x", Kind: ledger.AccountKind(9)},
		{ID: "01J0ACCOUNTCASH0000000000A", Name: "x", Kind: ledger.AccountCash, Initial: shared.NewMoney(0, shared.Currency("EUR"))},
	}
	for i, b := range bad {
		if err := b.Validate(); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("bad account %d accepted", i)
		}
	}

	builtin := &ledger.Account{ID: "01J0ACCOUNTPOT0000000000A", Name: "专注储蓄", BuiltinKey: ledger.BuiltinAccountFocusPot, Kind: ledger.AccountSavings, Initial: cny(0)}
	if err := builtin.Rename("x"); shared.Code(err) != "ledger.account.builtin" {
		t.Fatalf("builtin rename code = %q", shared.Code(err))
	}
	if err := builtin.Archive(); shared.Code(err) != "ledger.account.builtin" {
		t.Fatalf("builtin archive code = %q", shared.Code(err))
	}
}

func TestCategoryRules(t *testing.T) {
	c, err := ledger.NewCategory("01J0CATEGORYDINING0000000A", "餐饮", ledger.CategoryExpense)
	if err != nil {
		t.Fatalf("NewCategory: %v", err)
	}
	child, _ := ledger.NewCategory("01J0CATEGORYLUNCH000000000B", "午饭", ledger.CategoryExpense)
	if err := child.AttachParent(c); err != nil {
		t.Fatalf("AttachParent: %v", err)
	}
	grand, _ := ledger.NewCategory("01J0CATEGORYSUB0000000000C", "简餐", ledger.CategoryExpense)
	if err := grand.AttachParent(child); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a third level must be refused, got %v", err)
	}
	income, _ := ledger.NewCategory("01J0CATEGORYSALARY0000000A", "工资", ledger.CategoryIncome)
	if err := grand.AttachParent(income); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a direction mismatch must be refused, got %v", err)
	}
	if err := grand.AttachParent(nil); err != nil {
		t.Fatal(err)
	}

	self := &ledger.Category{ID: "01J0CATEGORYDINING0000000A", Name: "x", Direction: ledger.CategoryExpense, ParentID: "01J0CATEGORYDINING0000000A"}
	if !shared.IsKind(self.Validate(), shared.KindInvalid) {
		t.Fatal("a self parent must be invalid")
	}
	bad := []*ledger.Category{
		{Name: "x", Direction: ledger.CategoryExpense},
		{ID: "01J0CATEGORYDINING0000000A", Name: "", Direction: ledger.CategoryExpense},
		{ID: "01J0CATEGORYDINING0000000A", Name: "x", Direction: 9},
	}
	for i, b := range bad {
		if !shared.IsKind(b.Validate(), shared.KindInvalid) {
			t.Fatalf("bad category %d accepted", i)
		}
	}

	builtin := &ledger.Category{ID: "01J0CATEGORYAWARD0000000A", Name: "专注奖励", BuiltinKey: ledger.BuiltinCategoryFocusAward, Direction: ledger.CategoryIncome}
	if err := builtin.Rename("x"); shared.Code(err) != "ledger.category.builtin" {
		t.Fatalf("builtin rename code = %q", shared.Code(err))
	}
	if err := builtin.Rename("x"); !shared.IsKind(err, shared.KindConflict) {
		t.Fatal("renaming a builtin category must be a conflict")
	}
}

func TestBuiltinKeysAreUniqueAndPrefixed(t *testing.T) {
	seen := make(map[string]bool, len(ledger.AllBuiltinKeys))
	for _, k := range ledger.AllBuiltinKeys {
		if seen[k] {
			t.Fatalf("duplicate builtin key %q", k)
		}
		seen[k] = true
		if !strings.HasPrefix(k, "builtin.") {
			t.Fatalf("builtin key %q must start with builtin.", k)
		}
	}
}

func TestBudgetUsageAndRemaining(t *testing.T) {
	b, err := ledger.NewBudget("01J0BUDGET0000000000001", "01J0CATEGORYDINING0000000A", shared.MustParseMonth("2026-11"), cny(10000))
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}
	if got := b.Usage(cny(5000)); got != 0.5 {
		t.Fatalf("Usage = %v, want 0.5", got)
	}
	if got := b.Usage(cny(-5000)); got != 0.5 {
		t.Fatalf("Usage must use the absolute spent amount, got %v", got)
	}
	if got := b.Usage(shared.NewMoney(5000, shared.USD)); got != 0 {
		t.Fatalf("a currency mismatch must not produce a ratio, got %v", got)
	}
	if got := b.Remaining(cny(4000)).Minor(); got != 6000 {
		t.Fatalf("Remaining = %d, want 6000", got)
	}
	if got := b.Remaining(cny(12000)).Minor(); got != 0 {
		t.Fatalf("Remaining must floor at zero, got %d", got)
	}
	if got := b.Remaining(shared.NewMoney(1, shared.USD)).Minor(); got != 10000 {
		t.Fatalf("a currency mismatch must return the whole limit, got %d", got)
	}
	zero := &ledger.Budget{Limit: cny(0)}
	if got := zero.Usage(cny(100)); got != 0 {
		t.Fatalf("a zero limit must report 0, got %v", got)
	}

	bad := []*ledger.Budget{
		{CategoryID: "01J0CATEGORYDINING0000000A", Month: shared.MustParseMonth("2026-11"), Limit: cny(1)},
		{ID: "01J0BUDGET0000000000001", Month: shared.MustParseMonth("2026-11"), Limit: cny(1)},
		{ID: "01J0BUDGET0000000000001", CategoryID: "01J0CATEGORYDINING0000000A", Limit: cny(1)},
		{ID: "01J0BUDGET0000000000001", CategoryID: "01J0CATEGORYDINING0000000A", Month: shared.MustParseMonth("2026-11"), Limit: shared.NewMoney(1, shared.Currency("EUR"))},
		{ID: "01J0BUDGET0000000000001", CategoryID: "01J0CATEGORYDINING0000000A", Month: shared.MustParseMonth("2026-11"), Limit: cny(0)},
	}
	for i, b := range bad {
		if !shared.IsKind(b.Validate(), shared.KindInvalid) {
			t.Fatalf("bad budget %d accepted", i)
		}
	}
}

func TestLedgerParsingRoundTrips(t *testing.T) {
	for _, k := range []ledger.EntryKind{ledger.EntryExpense, ledger.EntryIncome, ledger.EntryTransfer} {
		if back, err := ledger.ParseEntryKind(k.String()); err != nil || back != k {
			t.Fatalf("entry kind round trip: %v %v", back, err)
		}
	}
	if _, err := ledger.ParseEntryKind("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown entry kind must be invalid, got %v", err)
	}
	if got := ledger.EntryKind(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	for _, o := range []ledger.Origin{ledger.OriginManual, ledger.OriginFocusReward} {
		if back, err := ledger.ParseOrigin(o.String()); err != nil || back != o {
			t.Fatalf("origin round trip: %v %v", back, err)
		}
	}
	if _, err := ledger.ParseOrigin("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown origin must be invalid, got %v", err)
	}
	if got := ledger.Origin(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	for _, k := range []ledger.AccountKind{ledger.AccountCash, ledger.AccountDebitCard, ledger.AccountWallet, ledger.AccountSavings, ledger.AccountCredit} {
		if back, err := ledger.ParseAccountKind(k.String()); err != nil || back != k {
			t.Fatalf("account kind round trip: %v %v", back, err)
		}
	}
	if _, err := ledger.ParseAccountKind("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown account kind must be invalid, got %v", err)
	}
	if got := ledger.AccountKind(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	for _, d := range []ledger.CategoryDirection{ledger.CategoryExpense, ledger.CategoryIncome} {
		if back, err := ledger.ParseCategoryDirection(d.String()); err != nil || back != d {
			t.Fatalf("direction round trip: %v %v", back, err)
		}
	}
	if _, err := ledger.ParseCategoryDirection("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown direction must be invalid, got %v", err)
	}
	if got := ledger.CategoryDirection(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
}

func mustEntry(t *testing.T, now time.Time, in ledger.EntryInput) *ledger.Entry {
	t.Helper()
	e, err := ledger.NewEntry("01J0ENTRY000000000000009", now, in)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return e
}

// mustErr fails the test when the call unexpectedly succeeded.
func mustErr(t *testing.T, e *ledger.Entry, err error) error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got entry %+v", e)
	}
	return err
}

// mustRejected asserts the input is refused and returns the error.
func mustRejected(t *testing.T, now time.Time, in ledger.EntryInput) error {
	t.Helper()
	e, err := ledger.NewEntry("01J0ENTRY000000000000002", now, in)
	if err == nil {
		t.Fatalf("expected the entry to be refused, got %+v", e)
	}
	return err
}
