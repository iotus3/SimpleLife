package bootstrap

import (
	"testing"

	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/infrastructure/idgen"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// seedFixture opens an empty database and returns what seeding needs.
func seedFixture(t *testing.T) (*sqlite.DB, *settingsapp.Service, shared.IDGen) {
	t.Helper()
	store, err := sqlite.OpenMemory(t.Name())
	if err != nil {
		t.Fatalf("open memory database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, settingsapp.New(store.Settings()), idgen.New()
}

func TestSeedFillsAFreshDatabase(t *testing.T) {
	store, settings, ids := seedFixture(t)
	if err := seed(store, ids, settings); err != nil {
		t.Fatalf("seed: %v", err)
	}

	accounts, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != len(builtinAccounts) {
		t.Fatalf("got %d accounts, want %d", len(accounts), len(builtinAccounts))
	}
	for index, spec := range builtinAccounts {
		if accounts[index].BuiltinKey != spec.key {
			t.Errorf("account %d has key %q, want %q", index, accounts[index].BuiltinKey, spec.key)
		}
		if accounts[index].Kind != spec.kind {
			t.Errorf("account %q has kind %v, want %v", spec.key, accounts[index].Kind, spec.kind)
		}
		if accounts[index].SortOrder != index {
			t.Errorf("account %q has order %d, want %d", spec.key, accounts[index].SortOrder, index)
		}
		if accounts[index].Initial.Currency() != shared.DefaultCurrency {
			t.Errorf("account %q is in %v, want the default currency", spec.key, accounts[index].Initial.Currency())
		}
		if !accounts[index].Initial.IsZero() {
			t.Errorf("account %q starts with %v, want zero", spec.key, accounts[index].Initial)
		}
	}

	categories, err := store.Categories().List()
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != len(builtinCategories) {
		t.Fatalf("got %d categories, want %d", len(categories), len(builtinCategories))
	}
	// The list is grouped by direction before it is ordered by sort order, so the
	// seeded order is checked per row rather than by position.
	bySpec := make(map[string]*ledger.Category, len(categories))
	for _, category := range categories {
		bySpec[category.BuiltinKey] = category
	}
	for index, spec := range builtinCategories {
		category, ok := bySpec[spec.key]
		if !ok {
			t.Errorf("category %q is missing", spec.key)
			continue
		}
		if category.Direction != spec.direction {
			t.Errorf("category %q has direction %v, want %v", spec.key, category.Direction, spec.direction)
		}
		if category.SortOrder != index {
			t.Errorf("category %q has order %d, want %d", spec.key, category.SortOrder, index)
		}
		if category.Name == "" || category.Name == spec.key {
			t.Errorf("category %q has no translated name: %q", spec.key, category.Name)
		}
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	store, settings, ids := seedFixture(t)
	first, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("a fresh database already has %d accounts", len(first))
	}
	for run := 0; run < 3; run++ {
		if err := seed(store, ids, settings); err != nil {
			t.Fatalf("seed run %d: %v", run, err)
		}
	}
	accounts, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != len(builtinAccounts) {
		t.Fatalf("after three runs there are %d accounts, want %d", len(accounts), len(builtinAccounts))
	}
	categories, err := store.Categories().List()
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != len(builtinCategories) {
		t.Fatalf("after three runs there are %d categories, want %d", len(categories), len(builtinCategories))
	}
}

func TestSeedKeepsTheNamesWhenTheUserRenamedARow(t *testing.T) {
	store, settings, ids := seedFixture(t)
	if err := seed(store, ids, settings); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accounts, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	renamed := accounts[0]
	renamed.Name = "Wallet"
	if err := store.Accounts().Update(renamed); err != nil {
		t.Fatalf("rename account: %v", err)
	}
	if err := seed(store, ids, settings); err != nil {
		t.Fatalf("seed again: %v", err)
	}
	again, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(again) != len(builtinAccounts) {
		t.Fatalf("got %d accounts, want %d", len(again), len(builtinAccounts))
	}
	if again[0].Name != "Wallet" {
		t.Errorf("the rename was overwritten: name is %q", again[0].Name)
	}
}

func TestSeedUsesTheStoredLanguageForNames(t *testing.T) {
	store, settings, ids := seedFixture(t)
	if err := settings.SetLanguage(settingsapp.LanguageEn); err != nil {
		t.Fatalf("set language: %v", err)
	}
	if err := seed(store, ids, settings); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accounts, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	want := i18n.T(i18n.En, i18n.MessageID(ledger.BuiltinAccountCash))
	if accounts[0].Name != want {
		t.Errorf("the cash account is named %q, want %q", accounts[0].Name, want)
	}
	if accounts[0].Name == i18n.T(i18n.ZhCN, i18n.MessageID(ledger.BuiltinAccountCash)) {
		t.Errorf("the name is still in Chinese: %q", accounts[0].Name)
	}
}

func TestSeedUsesTheDefaultCurrency(t *testing.T) {
	store, settings, ids := seedFixture(t)
	if err := settings.SetDefaultCurrency(shared.USD); err != nil {
		t.Fatalf("set currency: %v", err)
	}
	if err := seed(store, ids, settings); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accounts, err := store.Accounts().List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	for _, account := range accounts {
		if account.Initial.Currency() != shared.USD {
			t.Errorf("account %q is in %v, want USD", account.BuiltinKey, account.Initial.Currency())
		}
	}
}
