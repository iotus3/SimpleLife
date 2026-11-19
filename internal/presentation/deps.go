// Package presentation is the Viva user interface: it declares pages, holds the little state a
// page needs between paints, and calls the application services. It owns no rules.
package presentation

import (
	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/application/dashboard"
	exportapp "github.com/iotus3/simplelife/app/internal/application/export"
	"github.com/iotus3/simplelife/app/internal/application/focus"
	goalapp "github.com/iotus3/simplelife/app/internal/application/goal"
	ingestapp "github.com/iotus3/simplelife/app/internal/application/ingest"
	"github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/application/template"
	updateapp "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Deps is everything the interface is wired to. It is built once, by the
// composition root, and never looked up at run time: a page that needs a new
// service fails to build rather than failing at the moment a user clicks.
type Deps struct {
	Tasks     *task.Service
	Focus     *focus.Service
	Ledger    *ledger.Service
	Templates *template.Service
	Goals     *goalapp.Service
	Settings  *settings.Service
	Export    *exportapp.Service
	Dashboard *dashboard.Service
	// Update is the release feed, or nil when this build has none.
	Update *updateapp.Service
	// Restart starts the program an update has just put on disk. The composition root
	// owns it because the single-instance claim has to be released first.
	Restart func() error
	// Import reads an exported file back into the database.
	Import *ingestapp.Service
	// Decode turns a file's bytes into documents. It lives in the infrastructure
	// layer, so it arrives as a function, symmetric with Encode.
	Decode func(layout export.Layout, data []byte) ([]export.Document, error)

	Clock shared.Clock
	// Encode writes the materialised sheets into the bytes of one file. The
	// encoder lives in the infrastructure layer, so it arrives as a function: the
	// interface never imports a file format.
	Encode func(layout export.Layout, sheets []export.Sheet) ([]byte, error)
	// SystemLanguages is app.PreferredLanguages(), used when the language
	// preference is "follow the system".
	SystemLanguages []string
	// SystemAppearance is app.SystemAppearance, followed by the "system" appearance preference. It
	// is a function because the answer changes while the window is open; nil reads as light.
	SystemAppearance func() app.Appearance
	// Icon is the window's picture: what the taskbar, Alt+Tab and the title bar
	// show. It arrives as a value because the asset is not the interface's.
	Icon app.WindowIcon
	// Version, Commit and BuildDate are what the about page reports and what a bug
	// report quotes. The build script stamps all three; a source build has only the
	// version.
	Version   string
	Commit    string
	BuildDate string
	// DataDir is the folder the database lives in, shown on the about page.
	DataDir string
	// SecondLaunch reports every later start handed to this copy by the single-instance claim; the
	// window comes forward and says so.
	SecondLaunch <-chan struct{}
	// Notify shows a system notification for something the window cannot say, such
	// as "still running in the status area" while the window is not on screen. It
	// arrives as a function because the notifier lives in the infrastructure layer.
	Notify func(title, body string)
}
