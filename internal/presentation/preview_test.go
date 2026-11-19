package presentation

import (
	"testing"

	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	tplapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// A file of several tables holds several sheets, so the preview is one block per
// sheet: a preview of the first table alone says nothing about the rest of the file.
func TestPreviewNamesEverySheet(t *testing.T) {
	fixture := newFixture(t)
	if _, err := fixture.deps.Tasks.Create(taskapp.CreateInput{
		Title: "写周报", DueDate: fixture.state.today(),
	}); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := fixture.deps.Templates.Save(tplapp.SaveInput{
		Name: "晨间", Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: template.Task{Title: "写日记", Estimate: 1},
	}); err != nil {
		t.Fatalf("template: %v", err)
	}

	fixture.state.exportDraft.tables["templates"] = true
	if err := fixture.state.buildPreview(); err != nil {
		t.Fatalf("preview: %v", err)
	}
	sheets := fixture.state.exportDraft.previewSheets
	if len(sheets) != 2 {
		t.Fatalf("the preview holds %d sheets, want 2: %+v", len(sheets), sheets)
	}
	// Tables come in the export catalog's order, which is the order the sheets are
	// written in.
	if sheets[0].name != i18n.T(i18n.ZhCN, i18n.MsgExportSheetTasks) ||
		sheets[1].name != i18n.T(i18n.ZhCN, i18n.MsgExportSheetTemplates) {
		t.Fatalf("the sheets are %q and %q", sheets[0].name, sheets[1].name)
	}
	if sheets[0].total != 1 || len(sheets[0].rows) != 1 {
		t.Fatalf("the tasks sheet reports %d rows and shows %d", sheets[0].total, len(sheets[0].rows))
	}

	// The page says which sheet is which, and how many rows each holds.
	session := fixture.openWindowSize(t, appConfigSize(windowWidth, 1700))
	fixture.state.route = RouteData
	fixture.state.markAll()
	settle(t, session)
	for index, sheet := range sheets {
		want := i18n.T(i18n.ZhCN, i18n.MsgDataPreviewSheet, index+1, len(sheets), sheet.name)
		if !session.HasText(want) {
			t.Fatalf("the preview does not name sheet %d (%q); texts: %v", index+1, want, session.Texts())
		}
	}
}

// A sheet whose rows the range excluded still exists in the file, and the preview
// says so instead of leaving the block empty without a word.
func TestPreviewSaysWhenASheetIsEmpty(t *testing.T) {
	fixture := newFixture(t)
	fixture.state.exportDraft.tables["templates"] = true
	if err := fixture.state.buildPreview(); err != nil {
		t.Fatalf("preview: %v", err)
	}
	sheets := fixture.state.exportDraft.previewSheets
	if len(sheets) != 2 {
		t.Fatalf("the preview holds %d sheets, want 2", len(sheets))
	}
	if sheets[1].total != 0 || len(sheets[1].rows) != 0 {
		t.Fatalf("the empty templates sheet reports %+v", sheets[1])
	}

	session := fixture.openWindowSize(t, appConfigSize(windowWidth, 1700))
	fixture.state.route = RouteData
	fixture.state.markAll()
	settle(t, session)
	empty := i18n.T(i18n.ZhCN, i18n.MsgDataPreviewEmpty)
	if !session.HasText(empty) {
		t.Fatalf("an empty sheet is not explained; texts: %v", session.Texts())
	}
}
