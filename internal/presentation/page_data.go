package presentation

import (
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// dataPage is M10: choose tables, choose columns, choose a format, save the file.
// The same plan feeds the preview, and the file itself is written by the
// infrastructure encoder, so what the user saw is what they get.
func (s *State) dataPage(c ui.C) {
	s.pageHeader(c, icons.Download, i18n.MsgDataTitle, i18n.MsgNavDataSub, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgDataExport)).Icon(icons.Download).Clicked() {
			s.exportNow(c)
		}
	})
	ui.Row(c).FillWidth().Items(ui.Start).Gap(theme.Layout().Gap).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
			s.tablesCard(c)
			s.fieldsCard(c)
		})
		ui.Column(c).Width(360).Gap(theme.Layout().Gap).Children(func() {
			s.formatCard(c)
			s.exportHint(c)
		})
	})
	s.previewCardData(c)
	s.importCard(c)
}

// tablesCard ticks the tables to export.
func (s *State) tablesCard(c ui.C) {
	s.cardTitled(c, icons.Database, i18n.MsgDataStepTables, nil, func() {
		s.hint(c, i18n.T(s.lang, i18n.MsgDataStepTablesHint))
		selected := 0
		ui.Flow(c).FillWidth().Gap(14).Children(func() {
			for _, table := range export.DefaultTables() {
				key := table.Key
				on := s.exportDraft.tables[key]
				box := ui.Checkbox(c, &on, s.Sheet(key))
				if box.Changed() {
					s.exportDraft.tables[key] = on
					s.refreshPreview(c)
				}
				if on {
					selected++
				}
			}
		})
		s.hint(c, i18n.T(s.lang, i18n.MsgDataTableCount, selected))
	})
}

// fieldsCard ticks the columns of each selected table. Rows that identify a record
// are always exported and cannot be unticked.
func (s *State) fieldsCard(c ui.C) {
	s.cardTitled(c, icons.ListTodo, i18n.MsgDataStepFields, nil, func() {
		shown := 0
		for _, table := range export.DefaultTables() {
			if !s.exportDraft.tables[table.Key] {
				continue
			}
			shown++
			key := table.Key
			if shown > 1 {
				ui.Divider(c)
			}
			ui.Text(c, s.Sheet(key)).FontSize(13).Weight(gfx.WeightSemibold).
				Color(s.palette.Text)
			ui.Flow(c).FillWidth().Gap(14).Children(func() {
				for _, column := range table.Columns {
					name := column.Key
					on := s.exportDraft.columns[key][name]
					if column.Always {
						s.tag(c, s.Column(key, name), s.palette.Muted)
						s.exportDraft.columns[key][name] = true
						continue
					}
					flag := on
					box := ui.Checkbox(c, &flag, s.Column(key, name))
					if box.Changed() {
						s.exportDraft.columns[key][name] = flag
						s.refreshPreview(c)
					}
				}
			})
		}
		if shown == 0 {
			s.empty(c, i18n.MsgDataPreviewEmpty)
		}
	})
}

// formatCard picks the range, the currency filter and the file layout.
func (s *State) formatCard(c ui.C) {
	s.cardTitled(c, icons.Settings, i18n.MsgDataStepFormat, nil, func() {
		s.label(c, i18n.MsgDataRange)
		modes := []int{0, 1, 2}
		tabs(c, &s.exportDraft.rangeMode, modes, func(mode int) string {
			switch mode {
			case 1:
				return i18n.T(s.lang, i18n.MsgDataRangeMonth)
			case 2:
				return i18n.T(s.lang, i18n.MsgDataRangeCustom)
			}
			return i18n.T(s.lang, i18n.MsgDataRangeAll)
		})
		if s.exportDraft.rangeMode == 2 {
			ui.Row(c).Gap(8).Children(func() {
				s.dateField(c, dateFieldExportFrom, &s.exportDraft.from, i18n.MsgDataRangeFrom,
					format.Date(s.lang, s.today().AddDays(-30)))
				s.dateField(c, dateFieldExportTo, &s.exportDraft.to, i18n.MsgDataRangeTo,
					format.Date(s.lang, s.today()))
			})
		}
		s.label(c, i18n.MsgDataCurrency)
		currencies := []string{"", string(shared.CNY), string(shared.USD)}
		tabs(c, &s.exportDraft.currency, currencies, func(value string) string {
			if value == "" {
				return i18n.T(s.lang, i18n.MsgDataCurrencyAll)
			}
			return string(shared.Currency(value)) + " " + shared.Currency(value).Symbol()
		})
		layoutName := s.label(c, i18n.MsgDataLayout)
		layouts := []export.Layout{export.LayoutXLSX, export.LayoutCSV, export.LayoutMarkdown}
		ui.Select(c, &s.exportDraft.layout, layouts, func(layout export.Layout) string {
			switch layout {
			case export.LayoutCSV:
				return i18n.T(s.lang, i18n.MsgDataLayoutCSV)
			case export.LayoutMarkdown:
				return i18n.T(s.lang, i18n.MsgDataLayoutMD)
			}
			return i18n.T(s.lang, i18n.MsgDataLayoutXLSX)
		}).FillWidth().Label(layoutName)
		ui.Switch(c, &s.exportDraft.includeArchived, i18n.T(s.lang, i18n.MsgDataArchived))
	})
}

// previewCardData shows the first rows of every selected table, one block per sheet,
// so a multi-sheet workbook is not previewed as if it held only the first table.
func (s *State) previewCardData(c ui.C) {
	s.cardTitled(c, icons.ChartColumn, i18n.MsgDataPreview, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgDataPreviewRefresh)).Icon(icons.Play).
			Secondary().Small().Clicked() {
			s.refreshPreview(c)
		}
	}, func() {
		if s.exportDraft.previewError != nil {
			ui.Text(c, i18n.MessageFor(s.lang, s.exportDraft.previewError)).Color(s.palette.Expense)
			return
		}
		sheets := s.exportDraft.previewSheets
		if len(sheets) == 0 {
			s.empty(c, i18n.MsgDataPreviewEmpty)
			return
		}
		for index, sheet := range sheets {
			if index > 0 {
				ui.Divider(c)
			}
			s.previewSheetBlock(c, index, len(sheets), sheet)
		}
	})
}

// previewSheetBlock is one sheet of the preview: which sheet of how many it is, how
// many rows it holds, and its first rows with the chosen columns as headers.
func (s *State) previewSheetBlock(c ui.C, index, count int, sheet previewSheet) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
		s.iconChip(c, icons.Database, s.palette.Muted, s.palette.SurfaceSoft, 24, 13)
		ui.Text(c, i18n.T(s.lang, i18n.MsgDataPreviewSheet, index+1, count, sheet.name)).
			FontSize(13).Weight(gfx.WeightSemibold).Color(s.palette.Text).
			FillWidth().MaxLines(1).Ellipsis()
		s.tag(c, i18n.T(s.lang, s.lang.Plural(sheet.total, i18n.MsgDataPreviewRowOne, i18n.MsgDataPreviewRows), sheet.total),
			s.palette.Muted)
	})
	ui.Column(c).FillWidth().Gap(6).Pad(12).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
		Children(func() {
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				for _, header := range sheet.headers {
					ui.Text(c, header).FontSize(12).Weight(gfx.WeightSemibold).
						Color(s.palette.Muted).FillWidth().MaxLines(1).Ellipsis()
				}
			})
			if len(sheet.rows) == 0 {
				// A table that matches the range but holds nothing is worth saying:
				// its sheet still exists in the file.
				ui.Text(c, i18n.T(s.lang, i18n.MsgDataPreviewEmpty)).FontSize(12).
					Color(s.palette.Faint)
			}
			for _, row := range sheet.rows {
				ui.Row(c).FillWidth().Gap(12).Children(func() {
					for _, cell := range row {
						ui.Text(c, cell).FontSize(12).Color(s.palette.Text).FillWidth().
							MaxLines(1).Ellipsis()
					}
				})
			}
		})
	s.hint(c, i18n.T(s.lang, i18n.MsgDataColumnCount, len(sheet.headers)))
}

// exportHint says what the export button in the page header will write, so the
// one action of the page is explained where the choice is made.
func (s *State) exportHint(c ui.C) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.Start).Gap(10).Children(func() {
			s.iconChip(c, icons.Download, s.palette.Accent, s.palette.AccentSoft, 30, 15)
			s.hint(c, i18n.T(s.lang, i18n.MsgDataExportHint))
		})
	})
}

// refreshPreview recomputes the preview after a selection changed.
func (s *State) refreshPreview(c ui.C) {
	s.apply(c, RouteData, s.buildPreview)
}

// exportNow writes the file through the save dialog. The encode happens off the
// paint, and the sink is written from the callback.
func (s *State) exportNow(c ui.C) {
	c.After(func() {
		plan, err := s.plan()
		if err != nil {
			s.toastError(err)
			s.repaint()
			return
		}
		sheets, err := s.deps.Export.Build(plan, s)
		if err != nil {
			s.toastError(err)
			s.repaint()
			return
		}
		data, err := s.deps.Encode(plan.Layout, sheets)
		if err != nil {
			s.toastError(err)
			s.repaint()
			return
		}
		// CSV and Markdown hold one table per file, so several tables leave as a
		// zip; XLSX always leaves as one workbook.
		name := plan.FileName(s.today())
		if plan.Layout != export.LayoutXLSX && len(sheets) > 1 {
			name = trimExtension(name) + ".zip"
		}
		options := app.SaveFileOptions{
			Title:         i18n.T(s.lang, i18n.MsgDataExport),
			SuggestedName: name,
			Filters: []app.FileFilter{
				{Name: strings.ToUpper(plan.Layout.Extension()), Pattern: "*" + plan.Layout.Extension()},
			},
		}
		err = app.SaveFileAsync(options, func(sink *app.FileSink, err error) {
			if err != nil {
				s.toastError(err)
				s.repaint()
				return
			}
			writeErr := sink.Write(data, func(writeErr error) {
				if writeErr != nil {
					s.toastError(writeErr)
					s.repaint()
					return
				}
				closeErr := sink.Close(func(closeErr error) {
					if closeErr != nil {
						s.toastError(closeErr)
						s.repaint()
						return
					}
					s.toast(i18n.T(s.lang, i18n.MsgDataExported, name), ui.ToastSuccess)
					s.repaint()
				})
				if closeErr != nil {
					s.toastError(closeErr)
					s.repaint()
				}
			})
			if writeErr != nil {
				s.toastError(writeErr)
				s.repaint()
			}
		})
		if err != nil {
			s.toastError(err)
			s.repaint()
		}
	})
}

// trimExtension drops the extension of a generated file name.
func trimExtension(name string) string {
	if index := strings.LastIndex(name, "."); index > 0 {
		return name[:index]
	}
	return name
}
