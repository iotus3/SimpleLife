package presentation

import (
	"strconv"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	ingestapp "github.com/iotus3/simplelife/app/internal/application/ingest"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
)

// importDraft is the file the user picked and what became of it. It is written from the dialog's
// callback, so every access goes through importMu.
type importDraft struct {
	fileName  string
	layout    export.Layout
	documents []export.Document
	index     int // which sheet of a workbook
	table     string
	report    *ingestapp.Report
	failure   string // a message code, never a sentence
	busy      bool
}

// importSnapshot is the draft as one paint sees it.
type importSnapshot struct {
	draft  importDraft
	loaded bool
}

// importState copies the draft so a paint cannot be surprised by the callback of
// the file dialog changing it halfway through.
func (s *State) importState() importSnapshot {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	draft := s.importDraft
	if draft.documents != nil {
		draft.documents = append([]export.Document(nil), draft.documents...)
	}
	if draft.report != nil {
		copied := *draft.report
		copied.Failed = append([]ingestapp.Failure(nil), draft.report.Failed...)
		draft.report = &copied
	}
	return importSnapshot{draft: draft, loaded: draft.fileName != ""}
}

// setImportDraft replaces the draft under the lock.
func (s *State) setImportDraft(fn func(draft *importDraft)) {
	s.importMu.Lock()
	fn(&s.importDraft)
	s.importMu.Unlock()
}

// Header implements the importer's header port: the labels an export of this column
// carries, in both languages, so a file written by either interface imports.
func (s *State) Header(table, column string) []string {
	byColumn, ok := columnNames[table]
	if !ok {
		return nil
	}
	id, ok := byColumn[column]
	if !ok {
		return nil
	}
	return []string{i18n.T(i18n.ZhCN, id), i18n.T(i18n.En, id)}
}

// importCard is the other half of the data page: read a file this application wrote
// back into the database.
func (s *State) importCard(c ui.C) {
	state := s.importState()
	draft := state.draft
	s.cardTitled(c, icons.Inbox, i18n.MsgDataImport, nil, func() {
		s.hint(c, i18n.T(s.lang, i18n.MsgDataImportHint))
		ui.Row(c).FillWidth().Items(ui.Center).Gap(12).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgDataImportPick)).Icon(icons.Inbox).Clicked() {
				s.pickImportFile(c)
			}
			if state.loaded {
				s.rowHint(c, draft.fileName)
			}
		})
		if !state.loaded {
			return
		}
		if draft.failure != "" {
			ui.Text(c, i18n.MessageForCode(s.lang, draft.failure, "")).
				FontSize(12.5).Color(s.palette.Expense).MaxLines(3)
			return
		}
		s.importControls(c, draft)
	})
}

// importControls asks which table the file is, then runs the import.
func (s *State) importControls(c ui.C, draft importDraft) {
	document := draft.documents[draft.index]
	ui.Row(c).FillWidth().Items(ui.End).Gap(12).Children(func() {
		// A workbook holds several tables: the user picks the sheet, not the file.
		if len(draft.documents) > 1 {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				sheetName := s.label(c, i18n.MsgDataImportSheet)
				index := draft.index
				ui.Select(c, &index, sheetIndexes(len(draft.documents)), func(value int) string {
					return draft.documents[value].Name
				}).FillWidth().Label(sheetName)
				if index != draft.index {
					chosen := index
					s.setImportDraft(func(d *importDraft) {
						d.index = chosen
						d.report = nil
						d.table = s.tableForSheet(d.documents[chosen].Name)
					})
				}
			})
		}
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			tableName := s.label(c, i18n.MsgDataImportTable)
			table := draft.table
			ui.Select(c, &table, ingestapp.Tables(), func(key string) string { return s.Sheet(key) }).
				FillWidth().Label(tableName)
			if table != draft.table {
				chosen := table
				s.setImportDraft(func(d *importDraft) { d.table = chosen; d.report = nil })
			}
		})
		ui.Column(c).Gap(4).Children(func() {
			s.label(c, i18n.MsgDataImportRows)
			ui.Text(c, strconv.Itoa(len(document.Rows))).FontSize(13).Mono().Color(s.palette.Text)
		})
	})
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Children(func() {
		ui.Spacer(c)
		button := ui.Button(c, i18n.T(s.lang, i18n.MsgDataImportRun)).Icon(icons.Check)
		if draft.busy {
			button = button.Disabled(true)
		}
		if button.Clicked() {
			s.runImport(c)
		}
	})
	s.importReport(c, draft)
}

// importReport shows what the last import did, and why the failures failed.
func (s *State) importReport(c ui.C, draft importDraft) {
	if draft.report == nil {
		return
	}
	report := draft.report
	tone := s.palette.Income
	if len(report.Failed) > 0 {
		tone = s.palette.Warning
	}
	ui.Text(c, i18n.T(s.lang, i18n.MsgDataImportDone, report.Created, report.Skipped, len(report.Failed))).
		FontSize(13).Color(tone).MaxLines(2)
	const shownMax = 5
	for index, failure := range report.Failed {
		if index == shownMax {
			ui.Text(c, i18n.T(s.lang, i18n.MsgDataImportMore, len(report.Failed)-shownMax)).
				FontSize(11.5).Color(s.palette.Muted)
			break
		}
		ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
			s.tag(c, "#"+strconv.Itoa(failure.Row), s.palette.Muted)
			ui.Text(c, i18n.MessageForCode(s.lang, failure.Code, "")).FontSize(12).
				Color(s.palette.Muted).FillWidth().MaxLines(1).Ellipsis()
		})
	}
}

// tableForSheet recognises the table from a sheet name, so a workbook this
// application exported needs no manual choice. An unknown name keeps the current
// table.
func (s *State) tableForSheet(name string) string {
	for _, key := range ingestapp.Tables() {
		if strings.EqualFold(s.Sheet(key), strings.TrimSpace(name)) {
			return key
		}
	}
	return s.importState().draft.table
}

// sheetIndexes is the sheet selector's value list.
func sheetIndexes(count int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = i
	}
	return out
}

// pickImportFile opens the file dialog and decodes what the user chose off the paint.
func (s *State) pickImportFile(c ui.C) {
	c.After(func() {
		options := app.OpenFileOptions{
			Title: i18n.T(s.lang, i18n.MsgDataImportPick),
			Filters: []app.FileFilter{
				{Name: "CSV", Pattern: "*.csv"},
				{Name: "Markdown", Pattern: "*.md *.markdown"},
				{Name: "Excel", Pattern: "*.xlsx"},
			},
		}
		app.OpenFilesAsync(options, func(files []app.FileRef, err error) {
			if err != nil {
				s.toastError(err)
				s.repaint()
				return
			}
			if len(files) == 0 {
				return
			}
			file := files[0]
			file.ReadAll(func(data []byte, readErr error) {
				if readErr != nil {
					s.setImportDraft(func(draft *importDraft) {
						*draft = importDraft{fileName: file.Name(), failure: shared.Code(readErr)}
						if draft.failure == "" {
							draft.failure = string(i18n.CodeImportFileType)
						}
					})
					s.repaint()
					return
				}
				s.decodeImport(file.Name(), data)
				s.repaint()
			})
		})
	})
}

// decodeImport stores the decoded documents, choosing the table the first sheet
// names.
func (s *State) decodeImport(name string, data []byte) {
	layout, ok := layoutOfName(name)
	if !ok {
		s.setImportDraft(func(draft *importDraft) {
			*draft = importDraft{fileName: name, failure: string(i18n.CodeImportFileType)}
		})
		return
	}
	documents, err := s.deps.Decode(layout, data)
	if err != nil {
		s.setImportDraft(func(draft *importDraft) {
			*draft = importDraft{fileName: name, layout: layout, failure: shared.Code(err)}
		})
		return
	}
	s.setImportDraft(func(draft *importDraft) {
		*draft = importDraft{fileName: name, layout: layout, documents: documents}
		draft.table = s.tableForSheet(documents[0].Name)
	})
}

// runImport imports the chosen table off the paint and repaints with the report.
func (s *State) runImport(c ui.C) {
	state := s.importState()
	if !state.loaded || len(state.draft.documents) == 0 {
		return
	}
	document := state.draft.documents[state.draft.index]
	table := state.draft.table
	if table == "" {
		table = ingestapp.Tables()[0]
	}
	s.setImportDraft(func(draft *importDraft) { draft.busy = true })
	c.After(func() {
		report, err := s.deps.Import.Import(table, document, s)
		s.setImportDraft(func(draft *importDraft) {
			draft.busy = false
			if err != nil {
				draft.failure = shared.Code(err)
				draft.report = nil
				return
			}
			draft.failure = ""
			draft.report = &report
		})
		switch {
		case err != nil:
			s.toast(i18n.MessageFor(s.lang, err), ui.ToastError)
		case report.Created > 0:
			s.toast(i18n.T(s.lang, i18n.MsgDataImportDone, report.Created, report.Skipped, len(report.Failed)), ui.ToastSuccess)
			s.markAll()
		default:
			s.toast(i18n.T(s.lang, i18n.MsgDataImportNothing), ui.ToastInfo)
		}
		s.repaint()
	})
}

// layoutOfName reads the format from the file name, which is what the dialog
// filtered on.
func layoutOfName(name string) (export.Layout, bool) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".csv"):
		return export.LayoutCSV, true
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".markdown"):
		return export.LayoutMarkdown, true
	case strings.HasSuffix(lower, ".xlsx"):
		return export.LayoutXLSX, true
	}
	return 0, false
}
