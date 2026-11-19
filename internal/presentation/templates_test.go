package presentation

import (
	"testing"

	tplapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// "Preview today" is a look, not a place: the same button puts it away, and a day
// with nothing to generate still answers with the card.
func TestTemplatePreviewClosesOnASecondPress(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTemplates
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if _, err := fixture.deps.Templates.Save(tplapp.SaveInput{
		Name: "每日复盘", Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: template.Task{Title: "写下今天的一件好事"},
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	if err := session.ClickLabel(zh(i18n.MsgTemplatePreview)); err != nil {
		t.Fatalf("preview today: %v", err)
	}
	settle(t, session)
	if !fixture.state.templatePreviewOpen {
		t.Fatal("the preview did not open")
	}
	if !session.HasText("写下今天的一件好事") {
		t.Fatalf("the preview shows nothing: %v", session.Texts())
	}

	// While the preview is open the button offers to put it away, and does.
	if err := session.ClickLabel(zh(i18n.MsgTemplatePreviewClose)); err != nil {
		t.Fatalf("the preview cannot be put away: %v", err)
	}
	settle(t, session)
	if fixture.state.templatePreviewOpen {
		t.Fatal("the preview stayed open after the second press")
	}
	// The template's own card keeps naming the todo it writes, so the mark of the
	// preview is its tag, not the title.
	if session.HasText(zh(i18n.MsgTemplatePreviewNew)) {
		t.Errorf("the preview card is still painted: %v", session.Texts())
	}
	if err := session.ClickLabel(zh(i18n.MsgTemplatePreview)); err != nil {
		t.Fatalf("the button does not offer the preview again: %v", err)
	}
}

// A day whose rules fire nothing still answers: an empty preview is a fact about the
// day, and silence would read as a broken button.
func TestTemplatePreviewSaysWhenTodayIsEmpty(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTemplates
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if err := session.ClickLabel(zh(i18n.MsgTemplatePreview)); err != nil {
		t.Fatalf("preview today: %v", err)
	}
	settle(t, session)
	if !fixture.state.templatePreviewOpen || len(fixture.state.templatePreview) != 0 {
		t.Fatalf("an empty preview did not open: open=%t items=%d",
			fixture.state.templatePreviewOpen, len(fixture.state.templatePreview))
	}
	if !session.HasText(zh(i18n.MsgTemplatePreviewEmpty)) {
		t.Errorf("an empty preview says nothing: %v", session.Texts())
	}
}

// The template editor carries the length of one pomodoro for the task it writes: the
// field sits beside the estimate, saving it stores it, and reopening the template
// shows the stored number rather than an empty box.
func TestTemplateEditorKeepsThePomodoroLength(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTemplates
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if _, err := fixture.deps.Templates.Save(tplapp.SaveInput{
		Name: "精读文献", Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: template.Task{Title: "精读一篇", Estimate: 2, FocusMinutes: 40},
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	// The card names the length, so a template that sets one is visible without
	// opening the editor.
	if !session.HasText(zh(i18n.MsgTemplateFocusLength, 40)) {
		t.Fatalf("the card does not show the length: %v", session.Texts())
	}
	if err := session.ClickLabel(zh(i18n.MsgTemplateEdit)); err != nil {
		t.Fatalf("open the editor: %v", err)
	}
	settle(t, session)
	if draft := fixture.state.templateDraft; draft == nil || draft.taskFocus != "40" {
		t.Fatalf("the editor did not show the stored length: %+v", fixture.state.templateDraft)
	}
	// Clear and retype: the field is the source of truth for the task it writes.
	if err := session.ClickLabel(zh(i18n.MsgTaskFieldPomodoro)); err != nil {
		t.Fatalf("click the length field: %v", err)
	}
	// Select the old number before typing: the field keeps its text, and typing
	// after a click appends to it.
	if err := session.Keys("cmd+a"); err != nil {
		t.Fatalf("select the length: %v", err)
	}
	if err := session.Type("35"); err != nil {
		t.Fatalf("type the length: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	stored, err := fixture.deps.Templates.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 1 || stored[0].Task.FocusMinutes != 35 {
		t.Fatalf("stored templates = %+v", stored)
	}
	// A template without a length writes plain tasks: the field stays empty instead
	// of claiming a number nobody chose.
	if err := session.ClickLabel(zh(i18n.MsgTemplateNew)); err != nil {
		t.Fatalf("open a new template: %v", err)
	}
	settle(t, session)
	if draft := fixture.state.templateDraft; draft == nil || draft.taskFocus != "" {
		t.Fatalf("a new template starts with a length: %+v", fixture.state.templateDraft)
	}
}
