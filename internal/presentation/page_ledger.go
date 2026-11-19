package presentation

import (
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	lapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// ledgerPage is M4: the month's entries, the accounts behind them and the budgets
// that watch them. Currencies are never mixed: every total is shown per currency.
func (s *State) ledgerPage(c ui.C) {
	s.pageHeader(c, icons.Wallet, i18n.MsgNavLedger, i18n.MsgNavLedgerSub, func() {
		s.monthStepper(c)
		if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerNewEntry)).Icon(icons.Plus).Clicked() {
			s.openEntryEditor("")
		}
	})

	s.ledgerSummary(c)
	tabs := []int{0, 1, 2, 3}
	pageTabs(c, &s.ledgerTab, tabs, func(index int) string {
		switch index {
		case 1:
			return i18n.T(s.lang, i18n.MsgLedgerTabEntries)
		case 2:
			return i18n.T(s.lang, i18n.MsgLedgerTabAccounts)
		case 3:
			return i18n.T(s.lang, i18n.MsgLedgerTabBudgets)
		}
		return i18n.T(s.lang, i18n.MsgLedgerTabCategories)
	})

	switch s.ledgerTab {
	case 1:
		s.entriesTab(c)
	case 2:
		s.accountsTab(c)
	case 3:
		s.budgetsTab(c)
	default:
		s.categoriesTab(c)
	}
	s.entryEditor(c)
	s.accountEditor(c)
	s.budgetEditor(c)
	s.categoryEditor(c)
}

// monthStepper is the month the page counts in, with the two arrows that move it.
func (s *State) monthStepper(c ui.C) {
	ui.Row(c).Items(ui.Center).Gap(2).PadSides(4, 4, 4, 4).
		Background(s.palette.Surface).Border(1, s.palette.Line).
		Radius(theme.Layout().RadiusControl).Children(func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerMonthPrev)).Icon(icons.ChevronLeft).
			IconOnly().Ghost().Height(iconSize).Clicked() {
			s.ledgerMonth = s.ledgerMonth.AddMonths(-1)
			s.markAll()
			s.repaint()
		}
		ui.Text(c, format.MonthLabel(s.lang, s.ledgerMonth)).FontSize(12.5).
			Weight(gfx.WeightMedium).Color(s.palette.Text).PadSides(6, 6, 6, 6)
		if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerMonthNext)).Icon(icons.ChevronRight).
			IconOnly().Ghost().Height(iconSize).Clicked() {
			s.ledgerMonth = s.ledgerMonth.AddMonths(1)
			s.markAll()
			s.repaint()
		}
	})
}

// ledgerSummary shows income, expense and net for the month, one line per
// currency.
func (s *State) ledgerSummary(c ui.C) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			s.summaryColumn(c, icons.ArrowUpRight, i18n.MsgLedgerSumIncome, s.entrySummary.Income,
				s.palette.Income)
			s.summaryColumn(c, icons.ArrowDownRight, i18n.MsgLedgerSumExpense, s.entrySummary.Expense,
				s.palette.Expense)
			s.summaryColumn(c, icons.TrendingUp, i18n.MsgLedgerSumNet, s.entrySummary.Net,
				s.palette.Text)
			s.summaryColumn(c, icons.ListTodo, i18n.MsgTotal, nil, s.palette.Muted)
		})
		if len(s.entrySummary.Income) > 1 || len(s.entrySummary.Expense) > 1 {
			s.hint(c, i18n.T(s.lang, i18n.MsgLedgerMultiCurrency))
		}
	})
}

// summaryColumn is one figure of the month: its mark, its label and the amount.
func (s *State) summaryColumn(c ui.C, icon gfx.Icon, label i18n.MessageID, totals []shared.MoneyTotal, color ui.Color) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Pad(12).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
		Children(func() {
			s.iconChip(c, icon, color, s.palette.Surface, 30, 15)
			ui.Column(c).Gap(2).FillWidth().Children(func() {
				ui.Text(c, i18n.T(s.lang, label)).FontSize(11.5).Color(s.palette.Muted)
				value := format.TotalsIn(s.lang, totals, s.deps.Settings.DefaultCurrency())
				if totals == nil {
					value = i18n.T(s.lang, s.lang.Plural(s.entrySummary.EntryCount,
						i18n.MsgLedgerCountOne, i18n.MsgLedgerCount), s.entrySummary.EntryCount)
				}
				ui.Text(c, value).FontSize(15).Weight(gfx.WeightSemibold).Color(color).
					MaxLines(1).Ellipsis()
			})
		})
}

// entriesTab is the month's running list, newest first.
func (s *State) entriesTab(c ui.C) {
	if len(s.entries) == 0 {
		s.card(c, func() {
			s.empty(c, i18n.MsgLedgerEmpty)
		})
		return
	}
	s.card(c, func() {
		for index, entry := range s.entries {
			s.entryRow(c, entry, index == len(s.entries)-1)
		}
	})
}

// entryRow is one movement: its kind, what it was for, when, and how much.
func (s *State) entryRow(c ui.C, entry *ledger.Entry, last bool) {
	id := entry.ID
	tone := s.entryColor(entry)
	ui.Row(c).FillWidth().Items(ui.Center).Gap(12).PadY(6).Children(func() {
		s.iconChip(c, s.entryIcon(entry), tone, s.palette.SurfaceSoft, 34, 17)
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			ui.Text(c, s.entryTitle(entry)).FontSize(14).Color(s.palette.Text).
				MaxLines(1).Ellipsis()
			meta := s.accountName(entry.AccountID)
			if !entry.CategoryID.IsZero() {
				meta = s.categoryName(entry.CategoryID) + " · " + meta
			}
			ui.Text(c, meta).Color(s.palette.Faint).FontSize(12).MaxLines(1).Ellipsis()
		})
		ui.Text(c, entry.OccurredAt.Format("01-02")).Color(s.palette.Faint).FontSize(12).
			MaxLines(1)
		ui.Text(c, s.entryAmount(entry)).FontSize(14).Weight(gfx.WeightSemibold).
			Color(tone).MaxLines(1)
		if s.iconAction(c, icons.Pencil, i18n.MsgEdit) {
			s.openEntryEditor(id)
		}
		if s.iconAction(c, icons.Trash2, i18n.MsgDelete) {
			s.askDelete(confirmEntry, id)
		}
	})
	if !last {
		ui.Divider(c)
	}
}

// entryIcon is the mark of a movement: money out, money in, or a transfer.
func (s *State) entryIcon(entry *ledger.Entry) gfx.Icon {
	switch entry.Kind {
	case ledger.EntryExpense:
		return icons.ArrowDownRight
	case ledger.EntryIncome:
		return icons.ArrowUpRight
	}
	return icons.Coins
}

// categoriesTab lists the categories of both directions with the actions that belong to them,
// which is why it has a tab of its own.
func (s *State) categoriesTab(c ui.C) {
	s.cardTitled(c, icons.Tag, i18n.MsgLedgerTabCategories, func() {
		for _, direction := range []ledger.CategoryDirection{ledger.CategoryExpense, ledger.CategoryIncome} {
			chosen := direction
			label := i18n.T(s.lang, s.categoryDirectionMessage(chosen))
			if ui.Button(c, label).Icon(icons.Plus).Secondary().Small().Clicked() {
				s.openCategoryEditor("", chosen)
			}
		}
	}, func() {
		if len(s.categories) == 0 {
			s.empty(c, i18n.MsgLedgerCategoryEmpty)
			return
		}
		last := len(s.categories) - 1
		for index, category := range s.categories {
			s.categoryRow(c, category, index == last)
		}
	})
}

// categoryRow is one category: what it is called, which side it belongs to, and
// the two actions that change it.
func (s *State) categoryRow(c ui.C, category *ledger.Category, last bool) {
	id := category.ID
	income := category.Direction == ledger.CategoryIncome
	tone := s.palette.Expense
	if income {
		tone = s.palette.Income
	}
	ui.Row(c).FillWidth().Items(ui.Center).Gap(12).PadY(6).Children(func() {
		s.iconChip(c, icons.Tag, tone, s.palette.SurfaceSoft, 34, 17)
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			ui.Text(c, i18n.DisplayName(s.lang, category.Name, category.BuiltinKey)).
				FontSize(14).Color(s.palette.Text).MaxLines(1).Ellipsis()
			ui.Text(c, i18n.T(s.lang, s.categoryDirectionMessage(category.Direction))).
				Color(s.palette.Faint).FontSize(12).MaxLines(1)
		})
		if category.BuiltinKey != "" {
			s.tag(c, i18n.T(s.lang, i18n.MsgLedgerCategoryBuiltin), s.palette.Muted)
		}
		if s.iconAction(c, icons.Pencil, i18n.MsgLedgerCategoryEdit) {
			s.openCategoryEditor(id, category.Direction)
		}
		if s.iconAction(c, icons.Trash2, i18n.MsgDelete) {
			s.askDelete(confirmCategory, id)
		}
	})
	if !last {
		ui.Divider(c)
	}
}

// categoryDirectionMessage is the label of one side of the ledger.
func (s *State) categoryDirectionMessage(direction ledger.CategoryDirection) i18n.MessageID {
	if direction == ledger.CategoryIncome {
		return i18n.MsgLedgerCategoryIn
	}
	return i18n.MsgLedgerCategoryOut
}

// categoryEditor creates or renames a category. A new one picks its side here; an
// existing one keeps it, because moving a category across the ledger would leave
// its entries on the wrong side.
func (s *State) categoryEditor(c ui.C) {
	draft := s.categoryDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgLedgerCategoryNew)
	if !draft.id.IsZero() {
		title = i18n.T(s.lang, i18n.MsgLedgerCategoryEdit)
	}
	ui.Modal(c, &draft.open, title, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			s.field(c, &draft.name, i18n.MsgName, i18n.T(s.lang, i18n.MsgLedgerCategoryNew))
			if !draft.id.IsZero() {
				return
			}
			directionName := s.label(c, i18n.MsgLedgerCategoryDirection)
			directions := []ledger.CategoryDirection{ledger.CategoryExpense, ledger.CategoryIncome}
			ui.Select(c, &draft.direction, directions, func(value ledger.CategoryDirection) string {
				return i18n.T(s.lang, s.categoryDirectionMessage(value))
			}).Label(directionName)
		})
	}, func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteLedger, func() error {
					if err := s.saveCategory(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}

// accountsTab lists the accounts with their balance, and creates, edits or deletes
// one. The two actions live on the row, like everywhere else in the window: a list
// that can only be added to is a list the user has to live with.
func (s *State) accountsTab(c ui.C) {
	s.cardTitled(c, icons.Database, i18n.MsgLedgerTabAccounts, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerAccountNew)).Icon(icons.Plus).
			Secondary().Small().Clicked() {
			s.resetAccountDraft()
			s.accountDraft.open = true
		}
	}, func() {
		if len(s.balances) == 0 {
			s.empty(c, i18n.MsgLedgerAccountEmpty)
			return
		}
		for index, balance := range s.balances {
			account := balance.Account
			id := account.ID
			ui.Row(c).FillWidth().Items(ui.Center).Gap(12).PadY(6).Children(func() {
				s.iconChip(c, icons.Wallet, s.palette.Muted, s.palette.SurfaceSoft, 34, 17)
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					name := i18n.DisplayName(s.lang, account.Name, account.BuiltinKey)
					ui.Text(c, name).FontSize(14).Color(s.palette.Text).MaxLines(1).Ellipsis()
					meta := i18n.T(s.lang, s.accountKindMessage(account.Kind))
					if account.Archived {
						meta = meta + " · " + i18n.T(s.lang, i18n.MsgLedgerAccountArch)
					}
					ui.Text(c, meta).Color(s.palette.Faint).FontSize(12).MaxLines(1).Ellipsis()
				})
				if account.BuiltinKey != "" {
					s.tag(c, i18n.T(s.lang, i18n.MsgLedgerCategoryBuiltin), s.palette.Muted)
				}
				ui.Text(c, format.Money(balance.Balance)).FontSize(14).
					Weight(gfx.WeightSemibold).Color(s.palette.Text).MaxLines(1)
				if s.iconAction(c, icons.Pencil, i18n.MsgLedgerAccountEdit) {
					s.openAccountEditor(id)
				}
				// A seeded account is never deleted: the first-run seeding would put it back, and a row that
				// returns after being deleted reads as a bug. The button stays visible but disabled.
				if ui.Button(c, i18n.T(s.lang, i18n.MsgDelete)).Icon(icons.Trash2).
					IconOnly().Ghost().Height(iconSize).Disabled(account.BuiltinKey != "").Clicked() {
					s.askDelete(confirmAccount, id)
				}
			})
			if index < len(s.balances)-1 {
				ui.Divider(c)
			}
		}
	})
}

// budgetsTab shows the month's ceilings with what each has left, then the categories that spent
// without one.
func (s *State) budgetsTab(c ui.C) {
	s.cardTitled(c, icons.Target, i18n.MsgLedgerTabBudgets, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerBudgetNew)).Icon(icons.Plus).
			Secondary().Small().Clicked() {
			s.openBudgetEditor("", shared.Money{}, "")
		}
	}, func() {
		if len(s.budgetRows) == 0 {
			s.empty(c, i18n.MsgLedgerBudgetEmpty)
			return
		}
		for index, row := range s.budgetRows {
			s.budgetRow(c, row)
			if index < len(s.budgetRows)-1 {
				ui.Divider(c)
			}
		}
	})
}

// budgetRow is one category: what it cost, the ceiling (if any) as a bar, and the
// two actions that change the ceiling.
func (s *State) budgetRow(c ui.C, row lapp.BudgetRow) {
	category := row.Category
	categoryID := category.ID
	ui.Column(c).FillWidth().Gap(6).PadY(4).Children(func() {
		ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Children(func() {
			s.iconChip(c, icons.Tag, s.palette.Warning, s.palette.SurfaceSoft, 30, 15)
			ui.Text(c, i18n.DisplayName(s.lang, category.Name, category.BuiltinKey)).
				FontSize(14).Color(s.palette.Text).FillWidth().MaxLines(1).Ellipsis()
			ui.Text(c, format.Money(row.Spent)).FontSize(14).
				Weight(gfx.WeightSemibold).Color(s.palette.Text).MaxLines(1)
			if row.Budget == nil {
				// No ceiling yet: the pencil creates one for this category, which is
				// faster than opening the dialog and finding it in the list.
				if ui.Button(c, i18n.T(s.lang, i18n.MsgLedgerBudgetNew)).Icon(icons.Plus).
					Ghost().Small().Clicked() {
					s.openBudgetEditor("", shared.Money{}, categoryID)
				}
				return
			}
			id := row.Budget.ID
			limit := row.Budget.Limit
			if s.iconAction(c, icons.Pencil, i18n.MsgLedgerBudgetEdit) {
				s.openBudgetEditor(id, limit, categoryID)
			}
			if s.iconAction(c, icons.Trash2, i18n.MsgDelete) {
				s.askDelete(confirmBudget, id)
			}
		})
		if row.Budget == nil {
			s.hint(c, i18n.T(s.lang, i18n.MsgLedgerBudgetNone))
			return
		}
		usage := row.Budget.Usage(row.Spent)
		color := s.palette.Focus
		if usage >= 1 {
			color = s.palette.Expense
		} else if usage >= 0.8 {
			color = s.palette.Warning
		}
		s.bar(c, usage, color)
		s.hint(c, i18n.T(s.lang, i18n.MsgLedgerBudgetLimit)+" "+
			format.Money(row.Budget.Limit)+" · "+
			i18n.T(s.lang, i18n.MsgLedgerBudgetLeft)+" "+
			format.Money(row.Budget.Remaining(row.Spent)))
	})
}

// bar draws a fraction as a filled rectangle, the only shape a budget needs.
func (s *State) bar(c ui.C, fraction float64, color ui.Color) {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	width := float32(fraction)
	track := s.palette.Track
	ui.Row(c).FillWidth().Height(6).Draw(func(d *ui.DrawContext) {
		d.Canvas.Rounded(d.Rect, 3, track)
		if width <= 0 {
			return
		}
		filled := ui.Rect{X: d.Rect.X, Y: d.Rect.Y, Width: d.Rect.Width * width, Height: d.Rect.Height}
		d.Canvas.Rounded(filled, 3, color)
	})
}

// entryEditor is the modal behind "add entry" and "edit".
func (s *State) entryEditor(c ui.C) {
	draft := s.entryDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgLedgerNewEntryTitle)
	ui.Modal(c, &draft.open, title, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			kinds := []ledger.EntryKind{ledger.EntryExpense, ledger.EntryIncome, ledger.EntryTransfer}
			tabs(c, &draft.kind, kinds, func(kind ledger.EntryKind) string {
				return i18n.T(s.lang, s.entryKindMessage(kind))
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					s.field(c, &draft.amount, i18n.MsgAmount, "12.34")
				})
				s.dateField(c, dateFieldEntry, &draft.date, i18n.MsgDate,
					format.Date(s.lang, s.today()))
			})
			accountName := i18n.T(s.lang, i18n.MsgLedgerFieldAccount)
			ui.Text(c, accountName).Muted().FontSize(12)
			accountIDs := make([]shared.ID, 0, len(s.accounts))
			for _, account := range s.accounts {
				if !account.Archived {
					accountIDs = append(accountIDs, account.ID)
				}
			}
			ui.Select(c, &draft.account, accountIDs, func(id shared.ID) string {
				return s.accountName(id)
			}).Label(accountName)
			if draft.kind == ledger.EntryTransfer {
				toName := i18n.T(s.lang, i18n.MsgLedgerFieldTo)
				ui.Text(c, toName).Muted().FontSize(12)
				ui.Select(c, &draft.toAccount, accountIDs, func(id shared.ID) string {
					return s.accountName(id)
				}).Label(toName)
			} else {
				categoryName := i18n.T(s.lang, i18n.MsgLedgerFieldCategory)
				ui.Text(c, categoryName).Muted().FontSize(12)
				categoryIDs := make([]shared.ID, 0, len(s.categories))
				for _, category := range s.categories {
					if s.matchesDirection(category, draft.kind) {
						categoryIDs = append(categoryIDs, category.ID)
					}
				}
				ui.Select(c, &draft.category, categoryIDs, func(id shared.ID) string {
					return s.categoryName(id)
				}).Label(categoryName)
			}
			s.field(c, &draft.note, i18n.MsgNote, i18n.T(s.lang, i18n.MsgOptional))
		})
	}, func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				// The dialog closes only once the entry is stored, so a rejected
				// amount leaves the form as the user typed it.
				s.apply(c, RouteLedger, func() error {
					if err := s.saveEntry(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}

// accountEditor creates an account, or edits the name and kind of an existing one.
func (s *State) accountEditor(c ui.C) {
	draft := s.accountDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgLedgerAccountNew)
	if !draft.id.IsZero() {
		title = i18n.T(s.lang, i18n.MsgLedgerAccountEdit)
	}
	ui.Modal(c, &draft.open, title, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			if draft.builtin {
				// A seeded account carries a translation key instead of a name, so its
				// name is shown but not offered for editing: the language switch owns it.
				s.label(c, i18n.MsgName)
				ui.Text(c, draft.name).FontSize(13).Color(s.palette.Text).MaxLines(1)
				s.hint(c, i18n.T(s.lang, i18n.MsgLedgerAccountBuiltinName))
			} else {
				s.field(c, &draft.name, i18n.MsgName, i18n.T(s.lang, i18n.MsgLedgerAccountNew))
			}
			kinds := []ledger.AccountKind{
				ledger.AccountCash, ledger.AccountDebitCard, ledger.AccountWallet,
				ledger.AccountSavings, ledger.AccountCredit,
			}
			kindName := i18n.T(s.lang, i18n.MsgLedgerAccountKind)
			ui.Select(c, &draft.kind, kinds, func(kind ledger.AccountKind) string {
				return i18n.T(s.lang, s.accountKindMessage(kind))
			}).Label(kindName)
			// Currency and opening balance are fixed once the account exists: changing
			// either would reinterpret every entry already booked to it. So a new
			// account chooses them, an existing one only reads them.
			if draft.id.IsZero() {
				currencies := []shared.Currency{shared.CNY, shared.USD}
				currencyName := i18n.T(s.lang, i18n.MsgLedgerAccountCurrency)
				ui.Select(c, &draft.currency, currencies, func(currency shared.Currency) string {
					return string(currency) + " " + currency.Symbol()
				}).Label(currencyName)
				s.field(c, &draft.initial, i18n.MsgLedgerAccountBal, "0")
				return
			}
			s.label(c, i18n.MsgLedgerAccountCurrency)
			ui.Text(c, string(draft.currency)+" "+draft.currency.Symbol()).FontSize(13).
				Color(s.palette.Text).MaxLines(1)
		})
	}, func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteLedger, func() error {
					if err := s.saveAccount(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}

// budgetEditor creates a ceiling for a category of this month, or edits the limit
// of one that exists. An existing ceiling keeps its category: moving it would leave
// the old one behind and quietly create a second, which is not what "edit" means.
func (s *State) budgetEditor(c ui.C) {
	draft := s.budgetDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgLedgerBudgetNew)
	if draft.editing {
		title = i18n.T(s.lang, i18n.MsgLedgerBudgetEdit)
	}
	ui.Modal(c, &draft.open, title, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			if draft.editing {
				s.label(c, i18n.MsgCategory)
				ui.Text(c, s.categoryName(draft.category)).FontSize(13).
					Color(s.palette.Text).MaxLines(1)
			} else {
				budgetName := i18n.T(s.lang, i18n.MsgCategory)
				ui.Text(c, budgetName).Muted().FontSize(12)
				categoryIDs := make([]shared.ID, 0, len(s.categories))
				for _, category := range s.categories {
					if category.Direction != ledger.CategoryExpense {
						continue
					}
					// One ceiling per category per month: a category that already has
					// one is not offered again, it is edited from its row instead.
					if s.budgetOf(category.ID) != nil {
						continue
					}
					categoryIDs = append(categoryIDs, category.ID)
				}
				ui.Select(c, &draft.category, categoryIDs, func(id shared.ID) string {
					return s.categoryName(id)
				}).Label(budgetName)
			}
			s.field(c, &draft.limit, i18n.MsgLedgerBudgetLimit, "0")
		})
	}, func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteLedger, func() error {
					if err := s.saveBudget(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}

// budgetOf is the ceiling of a category in the month the page shows, if it has one.
func (s *State) budgetOf(category shared.ID) *ledger.Budget {
	for _, budget := range s.budgets {
		if budget.CategoryID == category {
			return budget
		}
	}
	return nil
}

// entryTitle is what a row says: the note, or the category and account.
func (s *State) entryTitle(entry *ledger.Entry) string {
	if entry.Note != "" {
		return entry.Note
	}
	if !entry.CategoryID.IsZero() {
		return s.categoryName(entry.CategoryID)
	}
	return i18n.T(s.lang, s.entryKindMessage(entry.Kind))
}

// entryAmount renders the amount with the sign of its kind.
func (s *State) entryAmount(entry *ledger.Entry) string {
	switch entry.Kind {
	case ledger.EntryExpense:
		return format.Money(entry.Amount.Neg())
	case ledger.EntryIncome:
		return "+" + format.Money(entry.Amount)
	}
	return format.Money(entry.Amount)
}

func (s *State) entryColor(entry *ledger.Entry) ui.Color {
	switch entry.Kind {
	case ledger.EntryExpense:
		return s.palette.Expense
	case ledger.EntryIncome:
		return s.palette.Income
	}
	return s.palette.Muted
}

func (s *State) matchesDirection(category *ledger.Category, kind ledger.EntryKind) bool {
	if kind == ledger.EntryIncome {
		return category.Direction == ledger.CategoryIncome
	}
	return category.Direction == ledger.CategoryExpense
}

// accountName resolves an account id, translating the built-in rows.
func (s *State) accountName(id shared.ID) string {
	for _, account := range s.accounts {
		if account.ID == id {
			return i18n.DisplayName(s.lang, account.Name, account.BuiltinKey)
		}
	}
	return i18n.T(s.lang, i18n.MsgNone)
}

// categoryName resolves a category id, translating the built-in rows.
func (s *State) categoryName(id shared.ID) string {
	for _, category := range s.categories {
		if category.ID == id {
			return i18n.DisplayName(s.lang, category.Name, category.BuiltinKey)
		}
	}
	return i18n.T(s.lang, i18n.MsgNone)
}
