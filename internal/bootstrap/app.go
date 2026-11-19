// Package bootstrap is the composition root: the only place that knows how the layers
// fit together.
package bootstrap

import (
	"os"
	"sync"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/app/singleinstance"

	"github.com/iotus3/simplelife/app/assets"
	dashboardapp "github.com/iotus3/simplelife/app/internal/application/dashboard"
	exportapp "github.com/iotus3/simplelife/app/internal/application/export"
	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	goalapp "github.com/iotus3/simplelife/app/internal/application/goal"
	ingestapp "github.com/iotus3/simplelife/app/internal/application/ingest"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	templateapp "github.com/iotus3/simplelife/app/internal/application/template"
	updateapp "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/buildinfo"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/infrastructure/clock"
	exportinfra "github.com/iotus3/simplelife/app/internal/infrastructure/export"
	"github.com/iotus3/simplelife/app/internal/infrastructure/idgen"
	"github.com/iotus3/simplelife/app/internal/infrastructure/notify"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
	"github.com/iotus3/simplelife/app/internal/infrastructure/power"
	updateinfra "github.com/iotus3/simplelife/app/internal/infrastructure/update"
	"github.com/iotus3/simplelife/app/internal/presentation"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// Run starts the application and blocks until the window closes.
func Run() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	// One copy per user, claimed before the database is opened: a later start hands its
	// command line to this copy and leaves. Buffered and coalesced, so a double-click that
	// arrives twice is one request to come to the front.
	launches := make(chan struct{}, 1)
	if !secondInstanceAllowed() {
		claim, primary, err := singleinstance.SingleInstance(instanceID, os.Args[1:], func([]string) {
			select {
			case launches <- struct{}{}:
			default:
			}
		})
		if err != nil {
			return err
		}
		if !primary {
			return nil
		}
		defer claim.Close()
	}

	store, err := sqlite.Open(config.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	wallClock := clock.System{}
	ids := idgen.New()

	settings := settingsapp.New(store.Settings())
	if err := seed(store, ids, settings); err != nil {
		return err
	}

	tasks := taskapp.New(store.Tasks(), store, wallClock, ids)
	money := ledgerapp.New(store.Entries(), store.Accounts(), store.Categories(), store.Budgets(), store, wallClock, ids)
	templates := templateapp.New(store.Templates(), store.Tasks(), store, wallClock, ids)
	goals := goalapp.New(store.Goals(), store, wallClock, ids)

	// The language the window is drawn in, shared with the adapters that speak to
	// the system out of band (a notification) and updated when the user switches.
	var language sync.Map
	setLanguage := func(lang i18n.Lang) { language.Store("lang", lang) }
	setLanguage(i18n.Resolve(settings.Language(), app.PreferredLanguages()))
	currentLanguage := func() i18n.Lang {
		if value, ok := language.Load("lang"); ok {
			if lang, ok := value.(i18n.Lang); ok {
				return lang
			}
		}
		return i18n.ZhCN
	}

	notifier := notify.Notifier{Translate: func(code string) string {
		lang := currentLanguage()
		if id := i18n.MessageID(code); i18n.Has(lang, id) {
			return i18n.T(lang, id)
		}
		return code
	}}
	focus := focusapp.New(
		store.Sessions(), store.Tasks(), store.Entries(), money,
		store, wallClock, ids, settings, notifier, power.Guard{},
	)
	board := dashboardapp.New(tasks, focus, money, wallClock)
	exporter := exportapp.New(
		store.Tasks(), store.Sessions(), store.Entries(), store.Accounts(),
		store.Categories(), store.Budgets(), store.Templates(), wallClock,
	)

	importer := ingestapp.New(
		store.Tasks(), store.Entries(), store.Accounts(), store.Categories(),
		store.Budgets(), store.Templates(), wallClock, ids,
	)

	// The release channel this build was pointed at. A source build has none, and
	// the about page says so instead of offering a check that cannot work.
	updates := updateapp.New(updateinfra.Fetcher{}, buildinfo.Version)

	commit, buildDate := buildinfo.Identity()

	deps := presentation.Deps{
		Icon:             assets.WindowIcon(),
		Tasks:            tasks,
		Focus:            focus,
		Ledger:           money,
		Templates:        templates,
		Goals:            goals,
		Settings:         settings,
		Export:           exporter,
		Dashboard:        board,
		Clock:            wallClock,
		SystemLanguages:  app.PreferredLanguages(),
		SystemAppearance: app.SystemAppearance,
		Update:           updates,
		Import:           importer,
		Decode:           exportinfra.Decode,
		Version:          buildinfo.Version,
		Commit:           commit,
		BuildDate:        buildDate,
		DataDir:          config.DataDir,
		SecondLaunch:     launches,
		Notify: func(title, body string) {
			// The notifier translates message codes; a sentence it does not know
			// passes through as it is.
			_ = notifier.Notify(shared.Notification{Title: title, Body: body})
		},
		Encode: exportinfra.Encode,
	}
	return presentation.Run(deps, presentation.WithLanguageObserver(setLanguage))
}

// instanceID names the single-instance claim. Reverse-DNS, and the same for every
// copy of the program: the identity that matters is "this program", not the folder a
// copy happens to sit in.
const instanceID = "dev.iotus3.simplelife"

// allowSecondEnv lets a caller opt out of the claim. It exists for two honest
// reasons: the smoke test must be able to run while the developer has the application
// open, and a support case may need a second copy beside a stuck one.
const allowSecondEnv = "SIMPLELIFE_ALLOW_SECOND_INSTANCE"

// secondInstanceAllowed reports whether the caller asked for a second copy on
// purpose. An empty value, "0" and "false" all mean "no".
func secondInstanceAllowed() bool {
	value := os.Getenv(allowSecondEnv)
	return value != "" && value != "0" && value != "false"
}

// seed writes the built-in accounts and categories the first time the database is
// opened. Each row keeps its message key, so a language switch renames it too.
func seed(store *sqlite.DB, ids shared.IDGen, settings *settingsapp.Service) error {
	accounts, err := store.Accounts().List(true)
	if err != nil {
		return err
	}
	categories, err := store.Categories().List()
	if err != nil {
		return err
	}
	lang := i18n.Resolve(settings.Language(), nil)
	if len(accounts) == 0 {
		currency := settings.DefaultCurrency()
		for index, spec := range builtinAccounts {
			account, err := newBuiltinAccount(ids.New(), lang, spec, currency)
			if err != nil {
				return err
			}
			account.SortOrder = index
			if err := store.Accounts().Create(account); err != nil {
				return err
			}
		}
	}
	if len(categories) == 0 {
		for index, spec := range builtinCategories {
			category, err := newBuiltinCategory(ids.New(), lang, spec)
			if err != nil {
				return err
			}
			category.SortOrder = index
			if err := store.Categories().Create(category); err != nil {
				return err
			}
		}
	}
	return nil
}
