// Package settings stores the user's preferences as small JSON documents under single
// keys, so adding a field never needs a migration.
package settings

import (
	"encoding/json"
	"errors"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Keys are the storage keys. They are stable and never translated.
const (
	KeyTimer      = "focus.timer"
	KeyEconomy    = "focus.economy"
	KeyLanguage   = "ui.language"
	KeyTheme      = "ui.theme"
	KeyAutoUpdate = "ui.auto_update"
	KeyCurrency   = "ledger.default_currency"
	KeyClose      = "ui.close"
	KeyTrayHint   = "ui.tray_hint"
)

// What the window's close button does. "exit" quits the application; "tray" keeps
// it running with its window hidden, so a running pomodoro survives the user
// tidying their desktop.
const (
	CloseExit = "exit"
	CloseTray = "tray"
)

// Language preference values. "auto" follows the operating system.
const (
	LanguageAuto = "auto"
	LanguageZhCN = "zh-CN"
	LanguageEn   = "en"
)

// Appearance preference values. "auto" follows the operating system.
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Store is the key-value port. A missing key returns shared.ErrNotFound so the
// service can distinguish "not set yet" from "set to the zero value".
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Service reads settings once and caches them: the timer and the economy are
// consulted on every frame, so a database read per access would be wasteful, and
// the values only change when the user saves the settings form.
type Service struct {
	store Store

	loaded     bool
	timer      focus.TimerSettings
	economy    focus.EconomySettings
	language   string
	theme      string
	autoUpdate bool
	currency   shared.Currency
	close      string
	trayHint   bool
}

// New builds the service. Nothing is read until the first accessor call, so a
// broken settings row cannot stop the app from starting.
func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) ensure() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.timer = focus.DefaultTimerSettings()
	s.economy = focus.DefaultEconomySettings(shared.DefaultCurrency)
	s.language = LanguageAuto
	s.theme = ThemeAuto
	s.currency = shared.DefaultCurrency
	s.close = CloseExit

	if raw, err := s.store.Get(KeyTimer); err == nil {
		var t focus.TimerSettings
		if json.Unmarshal([]byte(raw), &t) == nil && t.Validate() == nil {
			s.timer = t
		}
	}
	if raw, err := s.store.Get(KeyEconomy); err == nil {
		// shared.Money has unexported fields, so the stored document spells the
		// amount and the currency out (see economyDoc) instead of relying on the
		// JSON encoder to reach into the type.
		if e, derr := decodeEconomy(raw); derr == nil && e.Validate() == nil {
			s.economy = e
		}
	}
	if raw, err := s.store.Get(KeyLanguage); err == nil {
		if ValidLanguage(raw) {
			s.language = raw
		}
	}
	if raw, err := s.store.Get(KeyTheme); err == nil {
		if ValidTheme(raw) {
			s.theme = raw
		}
	}
	if raw, err := s.store.Get(KeyAutoUpdate); err == nil {
		s.autoUpdate = raw == "true"
	}
	if raw, err := s.store.Get(KeyClose); err == nil {
		if ValidClose(raw) {
			s.close = raw
		}
	}
	if raw, err := s.store.Get(KeyTrayHint); err == nil {
		s.trayHint = raw == "true"
	}
	if raw, err := s.store.Get(KeyCurrency); err == nil {
		if c := shared.Currency(raw); c.Valid() {
			s.currency = c
		}
	}
}

// Timer returns the cached timer settings.
func (s *Service) Timer() focus.TimerSettings {
	s.ensure()
	return s.timer
}

// SetTimer validates and stores the timer settings.
func (s *Service) SetTimer(t focus.TimerSettings) error {
	if err := t.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := s.store.Set(KeyTimer, string(raw)); err != nil {
		return err
	}
	s.ensure()
	s.timer = t
	return nil
}

// Economy returns the cached economy settings.
func (s *Service) Economy() focus.EconomySettings {
	s.ensure()
	return s.economy
}

// SetEconomy validates and stores the economy settings.
func (s *Service) SetEconomy(e focus.EconomySettings) error {
	if err := e.Validate(); err != nil {
		return err
	}
	raw, err := encodeEconomy(e)
	if err != nil {
		return err
	}
	if err := s.store.Set(KeyEconomy, raw); err != nil {
		return err
	}
	s.ensure()
	s.economy = e
	return nil
}

// Language returns the stored preference ("auto", "zh-CN" or "en").
func (s *Service) Language() string {
	s.ensure()
	return s.language
}

// SetLanguage stores the preference.
func (s *Service) SetLanguage(pref string) error {
	if !ValidLanguage(pref) {
		return shared.Invalid("settings.language", "language", "unknown language preference")
	}
	if err := s.store.Set(KeyLanguage, pref); err != nil {
		return err
	}
	s.ensure()
	s.language = pref
	return nil
}

// Theme returns the stored appearance preference ("auto", "light" or "dark").
func (s *Service) Theme() string {
	s.ensure()
	return s.theme
}

// SetTheme stores the appearance preference.
func (s *Service) SetTheme(pref string) error {
	if !ValidTheme(pref) {
		return shared.Invalid("settings.theme", "theme", "unknown appearance preference")
	}
	if err := s.store.Set(KeyTheme, pref); err != nil {
		return err
	}
	s.ensure()
	s.theme = pref
	return nil
}

// DefaultCurrency returns the currency a new account starts with.
func (s *Service) DefaultCurrency() shared.Currency {
	s.ensure()
	return s.currency
}

// SetDefaultCurrency stores the default currency.
func (s *Service) SetDefaultCurrency(c shared.Currency) error {
	if !c.Valid() {
		return shared.Invalid("settings.currency", "currency", "currency must be CNY or USD")
	}
	if err := s.store.Set(KeyCurrency, string(c)); err != nil {
		return err
	}
	s.ensure()
	s.currency = c
	return nil
}

// ValidLanguage reports whether pref is one of the three accepted values.
func ValidLanguage(pref string) bool {
	switch pref {
	case LanguageAuto, LanguageZhCN, LanguageEn:
		return true
	}
	return false
}

// AutoUpdate reports whether the application checks for a newer release when it
// starts. It is false until the user turns it on: nothing reaches the network
// without being asked for.
func (s *Service) AutoUpdate() bool {
	s.ensure()
	return s.autoUpdate
}

// SetAutoUpdate stores the preference.
func (s *Service) SetAutoUpdate(on bool) error {
	value := "false"
	if on {
		value = "true"
	}
	if err := s.store.Set(KeyAutoUpdate, value); err != nil {
		return err
	}
	s.ensure()
	s.autoUpdate = on
	return nil
}

// ValidTheme reports whether pref is one of the three accepted values.
func ValidTheme(pref string) bool {
	switch pref {
	case ThemeAuto, ThemeLight, ThemeDark:
		return true
	}
	return false
}

// CloseAction returns what closing the window does ("exit" or "tray").
func (s *Service) CloseAction() string {
	s.ensure()
	return s.close
}

// SetCloseAction stores what closing the window does.
func (s *Service) SetCloseAction(action string) error {
	if !ValidClose(action) {
		return shared.Invalid("settings.close", "close", "close action must be exit or tray")
	}
	if err := s.store.Set(KeyClose, action); err != nil {
		return err
	}
	s.ensure()
	s.close = action
	return nil
}

// TrayHintShown reports whether the user has been told that closing the window left the
// program in the status area. It is stored: once per installation, not once per launch.
func (s *Service) TrayHintShown() bool {
	s.ensure()
	return s.trayHint
}

// SetTrayHintShown records that the sentence has been shown.
func (s *Service) SetTrayHintShown() error {
	if err := s.store.Set(KeyTrayHint, "true"); err != nil {
		return err
	}
	s.ensure()
	s.trayHint = true
	return nil
}

// ValidClose reports whether action is one of the two accepted values.
func ValidClose(action string) bool {
	switch action {
	case CloseExit, CloseTray:
		return true
	}
	return false
}

// economyDoc is the stored form of the economy settings: shared.Money has no exported
// fields, so the amount and the currency are spelled out.
type economyDoc struct {
	Enabled              bool   `json:"enabled"`
	RateMinor            int64  `json:"rate_minor"`
	RateCurrency         string `json:"rate_currency"`
	PenaltyRatioPermille int    `json:"penalty_ratio_permille"`
	PenaltyMinMinutes    int    `json:"penalty_min_minutes"`
	RewardCapMinor       int64  `json:"reward_cap_minor"`
	PenaltyCapMinor      int64  `json:"penalty_cap_minor"`
	RewardAccountID      string `json:"reward_account_id"`
	RewardCategoryID     string `json:"reward_category_id"`
	PenaltyCategoryID    string `json:"penalty_category_id"`
}

func encodeEconomy(e focus.EconomySettings) (string, error) {
	doc := economyDoc{
		Enabled:              e.Enabled,
		RateMinor:            e.Rate.Minor(),
		RateCurrency:         string(e.Rate.Currency()),
		PenaltyRatioPermille: e.PenaltyRatioPermille,
		PenaltyMinMinutes:    e.PenaltyMinMinutes,
		RewardCapMinor:       e.RewardCapDaily.Minor(),
		PenaltyCapMinor:      e.PenaltyCapDaily.Minor(),
		RewardAccountID:      string(e.RewardAccountID),
		RewardCategoryID:     string(e.RewardCategoryID),
		PenaltyCategoryID:    string(e.PenaltyCategoryID),
	}
	raw, err := json.Marshal(doc)
	return string(raw), err
}

func decodeEconomy(raw string) (focus.EconomySettings, error) {
	var doc economyDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return focus.EconomySettings{}, err
	}
	currency := shared.Currency(doc.RateCurrency)
	if !currency.Valid() {
		return focus.EconomySettings{}, errors.New("settings: unsupported currency in focus economy")
	}
	return focus.EconomySettings{
		Enabled:              doc.Enabled,
		Rate:                 shared.NewMoney(doc.RateMinor, currency),
		PenaltyRatioPermille: doc.PenaltyRatioPermille,
		PenaltyMinMinutes:    doc.PenaltyMinMinutes,
		RewardCapDaily:       shared.NewMoney(doc.RewardCapMinor, currency),
		PenaltyCapDaily:      shared.NewMoney(doc.PenaltyCapMinor, currency),
		RewardAccountID:      shared.ID(doc.RewardAccountID),
		RewardCategoryID:     shared.ID(doc.RewardCategoryID),
		PenaltyCategoryID:    shared.ID(doc.PenaltyCategoryID),
	}, nil
}
