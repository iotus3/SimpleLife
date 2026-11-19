package settings_test

import (
	"testing"
	"time"

	app "github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestDefaultsWhenNothingIsStored(t *testing.T) {
	svc := app.New(store.New().Settings())
	if got := svc.Timer(); got != focus.DefaultTimerSettings() {
		t.Fatalf("timer defaults = %+v", got)
	}
	if got := svc.Economy(); got.Enabled || got.Rate.Minor() != 50 {
		t.Fatalf("economy defaults = %+v", got)
	}
	if got := svc.Language(); got != app.LanguageAuto {
		t.Fatalf("language default = %q", got)
	}
	if got := svc.DefaultCurrency(); got != shared.CNY {
		t.Fatalf("currency default = %v", got)
	}
}

func TestTimerRoundTrip(t *testing.T) {
	st := store.New().Settings()
	svc := app.New(st)
	want := focus.TimerSettings{FocusDuration: 50 * time.Minute, ShortBreak: 10 * time.Minute, LongBreak: 30 * time.Minute, LongBreakEvery: 3, AutoStartBreak: true}
	if err := svc.SetTimer(want); err != nil {
		t.Fatalf("SetTimer: %v", err)
	}
	if got := app.New(st).Timer(); got != want {
		t.Fatalf("a fresh service must read the stored value: %+v", got)
	}
	bad := focus.TimerSettings{FocusDuration: time.Second}
	if err := svc.SetTimer(bad); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an invalid timer must be refused, got %v", err)
	}
}

func TestEconomyRoundTripKeepsMoneyExact(t *testing.T) {
	st := store.New().Settings()
	svc := app.New(st)
	want := focus.DefaultEconomySettings(shared.USD)
	want.Enabled = true
	want.Rate = shared.NewMoney(125, shared.USD)
	want.PenaltyRatioPermille = 750
	want.RewardCapDaily = shared.NewMoney(999900, shared.USD)
	want.RewardAccountID = "00000000000000000000000021"
	want.RewardCategoryID = "00000000000000000000000022"
	want.PenaltyCategoryID = "00000000000000000000000023"
	if err := svc.SetEconomy(want); err != nil {
		t.Fatalf("SetEconomy: %v", err)
	}
	got := app.New(st).Economy()
	if got.Rate.Minor() != 125 || got.Rate.Currency() != shared.USD {
		t.Fatalf("rate = %d %v", got.Rate.Minor(), got.Rate.Currency())
	}
	if got.PenaltyRatioPermille != 750 || got.RewardCapDaily.Minor() != 999900 {
		t.Fatalf("economy = %+v", got)
	}
	if !got.Enabled || got.RewardAccountID != want.RewardAccountID {
		t.Fatalf("ids lost: %+v", got)
	}
	if got.PenaltyCapDaily.Currency() != shared.USD {
		t.Fatal("the penalty cap must stay in the rate currency")
	}

	invalid := focus.DefaultEconomySettings(shared.CNY)
	invalid.Rate = shared.NewMoney(0, shared.CNY)
	if err := svc.SetEconomy(invalid); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an invalid economy must be refused, got %v", err)
	}
}

func TestCorruptStoredValuesFallBackToDefaults(t *testing.T) {
	st := store.New()
	_ = st.Settings().Set(app.KeyEconomy, "{not json")
	_ = st.Settings().Set(app.KeyTimer, "{not json")
	_ = st.Settings().Set(app.KeyLanguage, "klingon")
	_ = st.Settings().Set(app.KeyCurrency, "EUR")
	svc := app.New(st.Settings())
	if got := svc.Timer(); got != focus.DefaultTimerSettings() {
		t.Fatalf("corrupt timer must fall back: %+v", got)
	}
	if got := svc.Economy(); got.Rate.Currency() != shared.DefaultCurrency {
		t.Fatalf("corrupt economy must fall back: %+v", got)
	}
	if got := svc.Language(); got != app.LanguageAuto {
		t.Fatalf("corrupt language must fall back: %q", got)
	}
	if got := svc.DefaultCurrency(); got != shared.DefaultCurrency {
		t.Fatalf("corrupt currency must fall back: %v", got)
	}
}

func TestLanguageAndCurrencyValidation(t *testing.T) {
	svc := app.New(store.New().Settings())
	for _, pref := range []string{app.LanguageAuto, app.LanguageZhCN, app.LanguageEn} {
		if err := svc.SetLanguage(pref); err != nil {
			t.Fatalf("SetLanguage(%q): %v", pref, err)
		}
		if got := svc.Language(); got != pref {
			t.Fatalf("Language = %q, want %q", got, pref)
		}
	}
	if err := svc.SetLanguage("fr"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unknown language must be invalid, got %v", err)
	}
	if err := svc.SetDefaultCurrency(shared.USD); err != nil {
		t.Fatalf("SetDefaultCurrency: %v", err)
	}
	if got := svc.DefaultCurrency(); got != shared.USD {
		t.Fatalf("currency = %v", got)
	}
	if err := svc.SetDefaultCurrency(shared.Currency("EUR")); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unsupported currency must be invalid, got %v", err)
	}
}

func TestValidLanguageCoversExactlyThreeValues(t *testing.T) {
	if !app.ValidLanguage(app.LanguageAuto) || !app.ValidLanguage(app.LanguageZhCN) || !app.ValidLanguage(app.LanguageEn) {
		t.Fatal("the three supported preferences must be valid")
	}
	for _, bad := range []string{"", "zh", "zh-CN ", "EN", "en-US"} {
		if app.ValidLanguage(bad) {
			t.Fatalf("%q must not be a valid preference", bad)
		}
	}
}

func TestThemeDefaultsToAutoAndRoundTrips(t *testing.T) {
	raw := store.New()
	svc := app.New(raw.Settings())
	// The first frame must already follow the desktop, so "auto" is the default.
	if got := svc.Theme(); got != app.ThemeAuto {
		t.Fatalf("default theme = %q, want %q", got, app.ThemeAuto)
	}
	for _, pref := range []string{app.ThemeLight, app.ThemeDark, app.ThemeAuto} {
		if err := svc.SetTheme(pref); err != nil {
			t.Fatalf("SetTheme(%q): %v", pref, err)
		}
		if got := svc.Theme(); got != pref {
			t.Fatalf("Theme = %q, want %q", got, pref)
		}
	}
	// A second service over the same store reads the stored value back.
	if err := svc.SetTheme(app.ThemeDark); err != nil {
		t.Fatal(err)
	}
	if got := app.New(raw.Settings()).Theme(); got != app.ThemeDark {
		t.Fatalf("reopened Theme = %q", got)
	}
	if err := svc.SetTheme("sepia"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unknown appearance must be invalid, got %v", err)
	}
	// A corrupted row falls back to the default instead of failing to start.
	if err := raw.Settings().Set(app.KeyTheme, "nonsense"); err != nil {
		t.Fatal(err)
	}
	if got := app.New(raw.Settings()).Theme(); got != app.ThemeAuto {
		t.Fatalf("a corrupt theme row must fall back to auto, got %q", got)
	}
}

func TestValidThemeCoversExactlyThreeValues(t *testing.T) {
	for _, good := range []string{app.ThemeAuto, app.ThemeLight, app.ThemeDark} {
		if !app.ValidTheme(good) {
			t.Fatalf("%q must be valid", good)
		}
	}
	for _, bad := range []string{"", "Auto", "dark ", "system", "lightmode"} {
		if app.ValidTheme(bad) {
			t.Fatalf("%q must not be a valid preference", bad)
		}
	}
}

func TestAutoUpdateIsOffUntilAskedFor(t *testing.T) {
	raw := store.New()
	svc := app.New(raw.Settings())
	if svc.AutoUpdate() {
		t.Fatal("a fresh profile must not contact the network on start")
	}
	if err := svc.SetAutoUpdate(true); err != nil {
		t.Fatal(err)
	}
	if !svc.AutoUpdate() {
		t.Fatal("AutoUpdate must remember the choice")
	}
	if got := app.New(raw.Settings()).AutoUpdate(); !got {
		t.Fatal("the choice must survive a restart")
	}
	if err := svc.SetAutoUpdate(false); err != nil {
		t.Fatal(err)
	}
	if svc.AutoUpdate() {
		t.Fatal("turning it off must stick")
	}
	// A row written by something else is read as off unless it says "true".
	if err := raw.Settings().Set(app.KeyAutoUpdate, "yes"); err != nil {
		t.Fatal(err)
	}
	if app.New(raw.Settings()).AutoUpdate() {
		t.Fatal("an unreadable value must read as off")
	}
}

// The close button quits unless the user asked it to keep the application running,
// and a value that is neither of the two is refused rather than guessed at.
func TestCloseActionDefaultsToQuittingAndRoundTrips(t *testing.T) {
	st := store.New().Settings()
	svc := app.New(st)
	if got := svc.CloseAction(); got != app.CloseExit {
		t.Fatalf("close action default = %q, want %q", got, app.CloseExit)
	}
	if err := svc.SetCloseAction(app.CloseTray); err != nil {
		t.Fatalf("store tray: %v", err)
	}
	if got := app.New(st).CloseAction(); got != app.CloseTray {
		t.Fatalf("close action after a restart = %q, want %q", got, app.CloseTray)
	}
	for _, invalid := range []string{"", "minimize", "TRAY"} {
		if err := svc.SetCloseAction(invalid); err == nil {
			t.Fatalf("close action %q was accepted", invalid)
		}
	}
	if got := svc.CloseAction(); got != app.CloseTray {
		t.Fatalf("a refused value changed the stored one: %q", got)
	}
}
