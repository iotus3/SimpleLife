package presentation

import (
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// settingsPage is M6: language, currency, the timer lengths and the focus economy.
func (s *State) settingsPage(c ui.C) {
	s.pageHeader(c, icons.Settings, i18n.MsgNavSettings, i18n.MsgNavSettingsSub, nil)
	ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
		s.generalCard(c)
		s.timerCard(c)
		s.economyCard(c)
	})
}

// generalCard switches the language, the appearance and the currency. Both the
// language and the appearance change the whole window at once.
func (s *State) generalCard(c ui.C) {
	s.cardTitled(c, icons.Settings, i18n.MsgSettingsGeneral, nil, func() {
		// One block per setting, stacked: the three-across layout this card used to have fits the short
		// Chinese labels and collides in English.
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				s.label(c, i18n.MsgSettingsLanguage)
				preferences := []string{i18n.PrefAuto, i18n.PrefZhCN, i18n.PrefEn}
				tabs(c, &s.langPref, preferences, func(pref string) string {
					switch pref {
					case i18n.PrefZhCN:
						return i18n.T(s.lang, i18n.MsgSettingsLangZh)
					case i18n.PrefEn:
						return i18n.T(s.lang, i18n.MsgSettingsLangEn)
					}
					return i18n.T(s.lang, i18n.MsgSettingsLangAuto)
				})
				s.hint(c, i18n.T(s.lang, i18n.MsgSettingsLangHint))
			})
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				s.label(c, i18n.MsgSettingsAppearance)
				pref := s.themePref
				preferences := []string{settings.ThemeAuto, settings.ThemeLight, settings.ThemeDark}
				tabs(c, &pref, preferences, func(value string) string {
					return i18n.T(s.lang, themeMessage(value))
				})
				if pref != s.themePref {
					chosen := pref
					s.setThemePref(chosen)
					s.apply(c, RouteSettings, func() error { return s.deps.Settings.SetTheme(chosen) })
				}
				s.hint(c, i18n.T(s.lang, i18n.MsgSettingsThemeHint))
			})
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				s.label(c, i18n.MsgSettingsCurrency)
				currency := s.deps.Settings.DefaultCurrency()
				currencies := []shared.Currency{shared.CNY, shared.USD}
				tabs(c, &currency, currencies, func(value shared.Currency) string {
					return string(value) + " " + value.Symbol()
				})
				if currency != s.deps.Settings.DefaultCurrency() {
					chosen := currency
					s.apply(c, RouteSettings, func() error { return s.deps.Settings.SetDefaultCurrency(chosen) })
					s.resetEconomyForm()
					s.resetAccountDraft()
				}
				s.hint(c, i18n.T(s.lang, i18n.MsgSettingsCurrHint))
			})
			// Only where a status area exists: on a desktop without one, closing the
			// window always quits, and a row that offered "keep running in the status
			// area" would be a choice that cannot be kept.
			if s.closeActionVisible() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					s.label(c, i18n.MsgSettingsClose)
					// The window's close button either ends the application or leaves it
					// in the status area, where a running pomodoro keeps counting.
					action := s.deps.Settings.CloseAction()
					choices := []string{settings.CloseExit, settings.CloseTray}
					tabs(c, &action, choices, func(value string) string {
						return i18n.T(s.lang, closeMessage(value))
					})
					if action != s.deps.Settings.CloseAction() {
						chosen := action
						s.apply(c, RouteSettings, func() error { return s.deps.Settings.SetCloseAction(chosen) })
					}
					s.hint(c, i18n.T(s.lang, i18n.MsgSettingsCloseHint))
				})
			}
		})
		if s.deps.Settings.Language() != s.langPref {
			chosen := s.langPref
			s.apply(c, RouteSettings, func() error { return s.deps.Settings.SetLanguage(chosen) })
			s.lang = i18n.Resolve(chosen, s.deps.SystemLanguages)
			if s.onLanguage != nil {
				s.onLanguage(s.lang)
			}
			s.markAll()
		}
	})
}

// themeMessage is the label of one appearance preference.
func themeMessage(pref string) i18n.MessageID {
	switch pref {
	case settings.ThemeLight:
		return i18n.MsgSettingsThemeLight
	case settings.ThemeDark:
		return i18n.MsgSettingsThemeDark
	}
	return i18n.MsgSettingsThemeSystem
}

// closeMessage is the label of one close-button behavior.
func closeMessage(action string) i18n.MessageID {
	if action == settings.CloseTray {
		return i18n.MsgSettingsCloseTray
	}
	return i18n.MsgSettingsCloseExit
}

// timerCard edits the four lengths and the two automatic starts.
func (s *State) timerCard(c ui.C) {
	s.cardTitled(c, icons.Timer, i18n.MsgSettingsTimer, nil, func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.timerForm.focus, i18n.MsgSettingsTimerFocus, "25")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.timerForm.short, i18n.MsgSettingsTimerShort, "5")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.timerForm.long, i18n.MsgSettingsTimerLong, "15")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.timerForm.every, i18n.MsgSettingsTimerEvery, "4")
			})
		})
		ui.Row(c).FillWidth().Items(ui.Center).Gap(24).Children(func() {
			ui.Switch(c, &s.timerForm.autoBreak, i18n.T(s.lang, i18n.MsgSettingsTimerAutoBreak))
			ui.Switch(c, &s.timerForm.autoFocus, i18n.T(s.lang, i18n.MsgSettingsTimerAutoFocus))
			ui.Spacer(c)
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteSettings, func() error {
					if err := s.saveTimerForm(); err != nil {
						return err
					}
					s.toast(i18n.T(s.lang, i18n.MsgSettingsSaved), ui.ToastSuccess)
					return nil
				})
			}
		})
	})
}

// economyCard edits the reward and fine formula inputs.
func (s *State) economyCard(c ui.C) {
	s.cardTitled(c, icons.Coins, i18n.MsgSettingsEconomy, nil, func() {
		s.hint(c, i18n.T(s.lang, i18n.MsgSettingsEcoHint))
		ui.Switch(c, &s.economyForm.enabled, i18n.T(s.lang, i18n.MsgSettingsEcoEnabled))
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.economyForm.rate, i18n.MsgSettingsEcoRate, "1.00")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.economyForm.ratio, i18n.MsgSettingsEcoRatio, "50")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.economyForm.minMinutes, i18n.MsgSettingsEcoMin, "5")
			})
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.economyForm.rewardCap, i18n.MsgSettingsEcoRewardCap, "10.00")
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				s.field(c, &s.economyForm.penaltyCap, i18n.MsgSettingsEcoPenaltyCap, "10.00")
			})
		})
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			accountName := s.label(c, i18n.MsgSettingsEcoRewardAcc)
			ui.Select(c, &s.economyForm.rewardAccount, s.accountIDs(), func(id shared.ID) string {
				return s.accountName(id)
			}).Label(accountName)
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				categoryName := s.label(c, i18n.MsgSettingsEcoRewardCat)
				ui.Select(c, &s.economyForm.rewardCategory, s.categoryIDs(ledger.CategoryIncome), func(id shared.ID) string {
					return s.categoryName(id)
				}).Label(categoryName)
			})
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				penaltyName := s.label(c, i18n.MsgSettingsEcoPenaltyCat)
				ui.Select(c, &s.economyForm.penaltyCategory, s.categoryIDs(ledger.CategoryExpense), func(id shared.ID) string {
					return s.categoryName(id)
				}).Label(penaltyName)
			})
		})
		ui.Row(c).FillWidth().Children(func() {
			ui.Spacer(c)
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteSettings, func() error {
					if err := s.saveEconomyForm(); err != nil {
						return err
					}
					s.toast(i18n.T(s.lang, i18n.MsgSettingsSaved), ui.ToastSuccess)
					return nil
				})
			}
		})
	})
}

// accountIDs lists the accounts a money setting can point at.
func (s *State) accountIDs() []shared.ID {
	out := make([]shared.ID, 0, len(s.accounts))
	for _, account := range s.accounts {
		if !account.Archived {
			out = append(out, account.ID)
		}
	}
	return out
}

// categoryIDs lists the categories of one direction.
func (s *State) categoryIDs(direction ledger.CategoryDirection) []shared.ID {
	out := make([]shared.ID, 0, len(s.categories))
	for _, category := range s.categories {
		if category.Direction == direction {
			out = append(out, category.ID)
		}
	}
	return out
}
