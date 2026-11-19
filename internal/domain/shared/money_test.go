package shared

import (
	"errors"
	"testing"
)

func TestParseMoneyAcceptsUserInput(t *testing.T) {
	cases := []struct {
		in   string
		cur  Currency
		want int64
	}{
		{"0", CNY, 0},
		{"1", CNY, 100},
		{"12.34", CNY, 1234},
		{"12.3", CNY, 1230},
		{".5", CNY, 50},
		{"0.10", CNY, 10},
		{"-3.5", CNY, -350},
		{"−3.5", CNY, -350}, // U+2212 minus, as typed on some keyboards
		{"+7", CNY, 700},
		{"1,234.50", CNY, 123450},
		{"¥1,234.50", CNY, 123450},
		{" 8.00 ", CNY, 800},
		{"1_000", CNY, 100000},
		{"0.01", USD, 1},
		{"99.99", USD, 9999},
	}
	for _, c := range cases {
		got, err := ParseMoney(c.in, c.cur)
		if err != nil {
			t.Fatalf("ParseMoney(%q, %s) returned error: %v", c.in, c.cur, err)
		}
		if got.Minor() != c.want {
			t.Errorf("ParseMoney(%q, %s) = %d minor, want %d", c.in, c.cur, got.Minor(), c.want)
		}
		if got.Currency() != c.cur {
			t.Errorf("ParseMoney(%q, %s) currency = %s", c.in, c.cur, got.Currency())
		}
	}
}

func TestParseMoneyRefusesRatherThanRounds(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"abc",
		"1.234", // more decimals than the currency has
		"1.2.3",
		"-",
		"1e5",
	}
	for _, in := range cases {
		if _, err := ParseMoney(in, CNY); err == nil {
			t.Errorf("ParseMoney(%q) should have failed", in)
		} else if !IsKind(err, KindInvalid) {
			t.Errorf("ParseMoney(%q) error kind = %v, want invalid", in, err)
		}
	}
}

func TestParseMoneyRejectsUnknownCurrency(t *testing.T) {
	if _, err := ParseMoney("1.00", Currency("RMB")); err == nil {
		t.Fatal("an unregistered currency must be refused")
	}
}

func TestMoneyArithmeticIsExact(t *testing.T) {
	a := MustParseMoney("0.10", CNY)
	b := MustParseMoney("0.20", CNY)
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Minor() != 30 {
		t.Fatalf("0.10 + 0.20 = %s, want 0.30", sum)
	}
	if sum.String() != "0.30" {
		t.Fatalf("String() = %q, want 0.30", sum.String())
	}
	diff, err := sum.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Equal(a) {
		t.Fatalf("0.30 - 0.20 = %s, want 0.10", diff)
	}
}

func TestMoneyRefusesMixedCurrencies(t *testing.T) {
	cny := MustParseMoney("10.00", CNY)
	usd := MustParseMoney("10.00", USD)
	if _, err := cny.Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add across currencies: err = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := cny.Sub(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Sub across currencies: err = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := SumMoney(CNY, cny, usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("SumMoney across currencies: err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMoneyStringRoundTrip(t *testing.T) {
	cases := []struct {
		minor int64
		want  string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{100, "1.00"},
		{-1, "-0.01"},
		{-1250, "-12.50"},
		{123456789, "1234567.89"},
	}
	for _, c := range cases {
		m := NewMoney(c.minor, CNY)
		if got := m.String(); got != c.want {
			t.Errorf("NewMoney(%d).String() = %q, want %q", c.minor, got, c.want)
		}
		back, err := ParseMoney(m.String(), CNY)
		if err != nil {
			t.Fatalf("re-parsing %q: %v", m.String(), err)
		}
		if back.Minor() != c.minor {
			t.Errorf("round trip %d -> %q -> %d", c.minor, m.String(), back.Minor())
		}
	}
}

func TestTallyKeepsCurrenciesApartInStableOrder(t *testing.T) {
	var tally Tally
	tally.Add(MustParseMoney("10.00", USD))
	tally.Add(MustParseMoney("20.50", CNY))
	tally.Add(MustParseMoney("5.00", USD))

	totals := tally.Totals()
	if len(totals) != 2 {
		t.Fatalf("got %d totals, want 2 (one per currency)", len(totals))
	}
	if totals[0].Currency != USD || totals[0].Amount.Minor() != 1500 {
		t.Errorf("first total = %+v, want USD 15.00 (first seen wins)", totals[0])
	}
	if totals[1].Currency != CNY || totals[1].Amount.Minor() != 2050 {
		t.Errorf("second total = %+v, want CNY 20.50", totals[1])
	}
}

func TestCurrencyValidation(t *testing.T) {
	for _, c := range []Currency{CNY, USD} {
		if !c.Valid() {
			t.Errorf("%s should be valid", c)
		}
		if c.Symbol() == "" {
			t.Errorf("%s should have a symbol", c)
		}
	}
	for _, c := range []Currency{"", "RMB", "cny", "EUR"} {
		if c.Valid() {
			t.Errorf("%q must not be valid", c)
		}
	}
}
