package focus

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Reward quality is permille, not a float: money arithmetic never touches float64,
// so a ratio is an integer too.
const (
	QualityPerfect          = 1000 // no interruption
	QualityOneBreak         = 800  // exactly one interruption
	QualityManyBreaks       = 600  // two or more
	penaltyRatioPermilleMax = 2000
	rateMinorMin            = 1
	rateMinorMax            = 10000
	penaltyMinMinutesMax    = 60
)

// Outcome reasons are stable codes, not sentences: the presentation layer
// translates them, so the domain never holds user-visible text.
const (
	ReasonDisabled     = "focus.economy.disabled"
	ReasonNotFocus     = "focus.economy.not_focus"
	ReasonNotSettled   = "focus.economy.not_settled"
	ReasonTooShort     = "focus.economy.too_short"
	ReasonCountUp      = "focus.economy.count_up"
	ReasonBelowMinimum = "focus.economy.below_minimum"
	ReasonCapReached   = "focus.economy.cap_reached"
)

// EconomySettings is the focus economy configuration. Every amount is in the
// currency of the reward account; there is no conversion anywhere.
type EconomySettings struct {
	Enabled bool

	// Rate is the value of one focused minute, e.g. ¥0.50 = NewMoney(50, CNY).
	Rate shared.Money
	// PenaltyRatioPermille weights the penalty; 500 means "half the planned value".
	PenaltyRatioPermille int
	// PenaltyMinMinutes is the grace period: abandoning before it costs nothing.
	PenaltyMinMinutes int
	// RewardCapDaily and PenaltyCapDaily cap the day's total movement. A cap of
	// zero means "no cap" only for the penalty side; the reward side keeps its
	// cap so a mis-set rate cannot mint money.
	RewardCapDaily  shared.Money
	PenaltyCapDaily shared.Money

	RewardAccountID   shared.ID
	RewardCategoryID  shared.ID
	PenaltyCategoryID shared.ID
}

// DefaultEconomySettings: half a yuan per minute, half penalty, a 5 minute grace
// period, and caps worth 8 (reward) and 4×0.5 (penalty) standard 25 minute
// pomodoros. The feature still starts disabled.
func DefaultEconomySettings(c shared.Currency) EconomySettings {
	rate := shared.NewMoney(50, c)
	return EconomySettings{
		Enabled:              false,
		Rate:                 rate,
		PenaltyRatioPermille: 500,
		PenaltyMinMinutes:    5,
		RewardCapDaily:       shared.NewMoney(8*25*rate.Minor(), c),
		PenaltyCapDaily:      shared.NewMoney(4*25*rate.Minor()*500/1000, c),
	}
}

// Validate checks the bounds the settings form must respect.
func (e EconomySettings) Validate() error {
	if !e.Rate.Currency().Valid() {
		return shared.Invalid("focus.economy.currency", "rate", "currency must be CNY or USD")
	}
	if e.Rate.Minor() < rateMinorMin || e.Rate.Minor() > rateMinorMax {
		return shared.Invalid("focus.economy.rate", "rate", "rate must be 0.01..100.00 per minute")
	}
	if e.PenaltyRatioPermille < 0 || e.PenaltyRatioPermille > penaltyRatioPermilleMax {
		return shared.Invalid("focus.economy.penalty_ratio", "penalty_ratio", "penalty ratio must be 0..2.0")
	}
	if e.PenaltyMinMinutes < 0 || e.PenaltyMinMinutes > penaltyMinMinutesMax {
		return shared.Invalid("focus.economy.penalty_min", "penalty_min_minutes", "grace period must be 0..60 minutes")
	}
	if e.RewardCapDaily.Minor() < 0 || e.PenaltyCapDaily.Minor() < 0 {
		return shared.Invalid("focus.economy.cap", "", "daily caps must not be negative")
	}
	if e.RewardCapDaily.Currency() != e.Rate.Currency() || e.PenaltyCapDaily.Currency() != e.Rate.Currency() {
		return shared.Invalid("focus.economy.cap_currency", "", "daily caps must use the same currency as the rate")
	}
	if e.Enabled {
		if e.RewardAccountID.IsZero() {
			return shared.Invalid("focus.economy.account", "reward_account", "reward account is required")
		}
		if e.RewardCategoryID.IsZero() || e.PenaltyCategoryID.IsZero() {
			return shared.Invalid("focus.economy.category", "reward_category", "reward and penalty categories are required")
		}
	}
	return nil
}

// Outcome is what the economy says about one settled session: a zero amount unless Reason
// is empty, and a message code the UI translates.
type Outcome struct {
	Amount        shared.Money
	Reason        string
	CappedByDaily bool
}

// Applicable reports whether an entry should be written at all.
func (o Outcome) Applicable() bool { return o.Amount.Minor() > 0 }

// QualityPermille is the reward multiplier for a settled session: a session the
// user interrupted pays less, which is the entire point of counting interruptions.
func QualityPermille(s *Session) int {
	switch {
	case s.Interruptions <= 0:
		return QualityPerfect
	case s.Interruptions == 1:
		return QualityOneBreak
	default:
		return QualityManyBreaks
	}
}

// divRoundHalfUp divides by 1000 rounding half away from zero. Written out rather
// than using math.Round so the money path never converts to float64.
func divRoundHalfUp(v int64) int64 {
	if v >= 0 {
		return (v + 500) / 1000
	}
	return -((-v + 500) / 1000)
}

// Reward computes what a completed session earns, clamped by what is left of the
// daily cap.
func (e EconomySettings) Reward(s *Session, rewardedToday shared.Money) (Outcome, error) {
	if !e.Enabled {
		return Outcome{Reason: ReasonDisabled}, nil
	}
	if s.Kind != KindFocus {
		return Outcome{Reason: ReasonNotFocus}, nil
	}
	if s.Phase != PhaseDone {
		return Outcome{Reason: ReasonNotSettled}, nil
	}
	if rewardedToday.Currency() != e.Rate.Currency() {
		return Outcome{}, shared.ErrCurrencyMismatch
	}
	minutes := s.ElapsedMinutes(s.EndedAt)
	if minutes < 1 {
		return Outcome{Reason: ReasonTooShort}, nil
	}
	raw := divRoundHalfUp(e.Rate.Minor() * int64(minutes) * int64(QualityPermille(s)))
	if raw <= 0 {
		return Outcome{Reason: ReasonTooShort}, nil
	}
	remaining := e.RewardCapDaily.Minor() - rewardedToday.Minor()
	if remaining <= 0 {
		return Outcome{Reason: ReasonCapReached, CappedByDaily: true}, nil
	}
	capped := false
	if raw > remaining {
		raw = remaining
		capped = true
	}
	return Outcome{Amount: shared.NewMoney(raw, e.Rate.Currency()), CappedByDaily: capped}, nil
}

// Penalty charges an abandoned countdown on its planned length, so quitting at minute 6 of 25
// costs what quitting at 24 does. A count-up session has no plan to break.
func (e EconomySettings) Penalty(s *Session, penalizedToday shared.Money) (Outcome, error) {
	if !e.Enabled {
		return Outcome{Reason: ReasonDisabled}, nil
	}
	if s.Kind != KindFocus {
		return Outcome{Reason: ReasonNotFocus}, nil
	}
	if s.Phase != PhaseCanceled {
		return Outcome{Reason: ReasonNotSettled}, nil
	}
	if penalizedToday.Currency() != e.Rate.Currency() {
		return Outcome{}, shared.ErrCurrencyMismatch
	}
	if s.Planned <= 0 {
		return Outcome{Reason: ReasonCountUp}, nil
	}
	if s.ElapsedMinutes(s.EndedAt) < e.PenaltyMinMinutes {
		return Outcome{Reason: ReasonBelowMinimum}, nil
	}
	plannedMinutes := int64(s.Planned / time.Minute)
	if plannedMinutes <= 0 {
		return Outcome{Reason: ReasonBelowMinimum}, nil
	}
	raw := divRoundHalfUp(e.Rate.Minor() * plannedMinutes * int64(e.PenaltyRatioPermille))
	if raw <= 0 {
		return Outcome{Reason: ReasonDisabled}, nil
	}
	remaining := e.PenaltyCapDaily.Minor() - penalizedToday.Minor()
	if remaining <= 0 {
		return Outcome{Reason: ReasonCapReached, CappedByDaily: true}, nil
	}
	capped := false
	if raw > remaining {
		raw = remaining
		capped = true
	}
	return Outcome{Amount: shared.NewMoney(raw, e.Rate.Currency()), CappedByDaily: capped}, nil
}
