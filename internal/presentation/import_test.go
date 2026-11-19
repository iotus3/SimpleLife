package presentation

import (
	"testing"

	ingestapp "github.com/iotus3/simplelife/app/internal/application/ingest"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// stageImport puts a decoded file into the draft, which is what the file dialog
// does after the user picks one.
func stageImport(t *testing.T, fixture *testFixture, name string, table string, data []byte) {
	t.Helper()
	layout, ok := layoutOfName(name)
	if !ok {
		t.Fatalf("%s has no layout", name)
	}
	documents, err := fixture.deps.Decode(layout, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	fixture.state.setImportDraft(func(draft *importDraft) {
		*draft = importDraft{fileName: name, layout: layout, documents: documents, table: table}
	})
}

func TestImportCardReadsAFileBackIn(t *testing.T) {
	fixture := newFixture(t)
	// The import card sits under the export cards, so the whole page has to be
	// painted for its buttons to exist.
	session := fixture.openWindowSize(t, appConfigSize(windowWidth, 1700))
	fixture.state.route = RouteData
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if !session.HasText(zh(i18n.MsgDataImport)) || !session.HasText(zh(i18n.MsgDataImportPick)) {
		t.Fatalf("the data page offers no import; texts: %v", session.Texts())
	}

	// A file with the raw column keys, which is what an unedited export carries.
	stageImport(t, fixture, "tasks.csv", "tasks", []byte(
		"id,title,status,estimate_pomodoros\n"+
			"00000000000000000000000001,写周报,todo,2\n"+
			"00000000000000000000000002,,todo,1\n"))
	settle(t, session)
	if !session.HasText("tasks.csv") {
		t.Fatalf("the chosen file is not shown; texts: %v", session.Texts())
	}
	if err := session.ClickLabel(zh(i18n.MsgDataImportRun)); err != nil {
		t.Fatalf("start the import: %v", err)
	}
	settle(t, session)

	tasks, err := fixture.store.Tasks().List(task.Filter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != "写周报" {
		t.Fatalf("tasks = %+v", tasks)
	}
	// The good row is reported as added and the blank one as failed, with its row
	// number, so the user can fix the file.
	if !session.HasText(zh(i18n.MsgDataImportDone, 1, 0, 1)) {
		t.Fatalf("the report is missing; texts: %v", session.Texts())
	}
	if !session.HasText("#3") {
		t.Fatalf("a failed row must be named by its number; texts: %v", session.Texts())
	}
}

func TestImportCardExplainsAnUnreadableFile(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteData
	fixture.state.markAll()
	settle(t, session)

	fixture.state.decodeImport("notes.txt", []byte("whatever"))
	settle(t, session)
	if !session.HasText(i18n.T(i18n.ZhCN, i18n.MsgDataImportUnknownFile)) {
		t.Fatalf("an unsupported file must be explained; texts: %v", session.Texts())
	}
}

func TestImportDefaultsToTheTableASheetNames(t *testing.T) {
	fixture := newFixture(t)
	// An export names its sheets in the interface's language; a workbook coming
	// back must need no manual picking.
	if got := fixture.state.tableForSheet(fixture.state.Sheet("entries")); got != "entries" {
		t.Fatalf("tableForSheet = %q, want entries", got)
	}
	if got := fixture.state.tableForSheet("不认识的表"); got != "" {
		t.Fatalf("an unknown sheet must keep the current table, got %q", got)
	}
}

func TestImportHeaderMatchesBothLanguages(t *testing.T) {
	fixture := newFixture(t)
	// A file exported in English must import into a Chinese window, and the other
	// way round.
	headers := fixture.state.Header("tasks", "due_date")
	if len(headers) != 2 {
		t.Fatalf("headers = %v", headers)
	}
	if headers[0] != i18n.T(i18n.ZhCN, i18n.MsgExportTasksDueDate) {
		t.Fatalf("Chinese header = %q", headers[0])
	}
	if headers[1] != i18n.T(i18n.En, i18n.MsgExportTasksDueDate) {
		t.Fatalf("English header = %q", headers[1])
	}
	if got := fixture.state.Header("tasks", "nope"); got != nil {
		t.Fatalf("an unknown column has no header, got %v", got)
	}
}

func TestImportLayoutOfName(t *testing.T) {
	cases := map[string]export.Layout{
		"tasks.csv": export.LayoutCSV,
		"账单.CSV":    export.LayoutCSV,
		"note.md":   export.LayoutMarkdown,
		"w.xlsx":    export.LayoutXLSX,
	}
	for name, want := range cases {
		if got, ok := layoutOfName(name); !ok || got != want {
			t.Fatalf("layoutOfName(%q) = %v %v", name, got, ok)
		}
	}
	if _, ok := layoutOfName("report.zip"); ok {
		t.Fatal("a zip is not an importable file")
	}
}

var _ = ingestapp.Tables
