package template

import (
	"strconv"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// ParseRule is the inverse of the machine form the export writes: "daily", "weekdays:1,5",
// "monthly:15", "dates:2026-12-25,...", "every_n_days:3:from:2026-11-17". An unreadable value
// is refused rather than guessed at.
func ParseRule(text string) (TriggerRule, error) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	switch kind {
	case "daily":
		if len(parts) != 1 {
			return TriggerRule{}, malformed(text)
		}
		return TriggerRule{Kind: TriggerDaily}, nil
	case "weekdays":
		if len(parts) != 2 {
			return TriggerRule{}, malformed(text)
		}
		var days []time.Weekday
		for _, field := range strings.Split(parts[1], ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			n, err := strconv.Atoi(field)
			if err != nil || n < 0 || n > 6 {
				return TriggerRule{}, malformed(text)
			}
			days = append(days, time.Weekday(n))
		}
		rule := TriggerRule{Kind: TriggerWeekdays, Weekdays: days}
		if err := rule.Validate(); err != nil {
			return TriggerRule{}, err
		}
		return rule, nil
	case "monthly":
		if len(parts) != 2 {
			return TriggerRule{}, malformed(text)
		}
		day, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return TriggerRule{}, malformed(text)
		}
		rule := TriggerRule{Kind: TriggerMonthly, DayOfMonth: day}
		if err := rule.Validate(); err != nil {
			return TriggerRule{}, err
		}
		return rule, nil
	case "dates":
		if len(parts) != 2 {
			return TriggerRule{}, malformed(text)
		}
		var dates []shared.LocalDate
		for _, field := range strings.Split(parts[1], ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			day, err := shared.ParseLocalDate(field)
			if err != nil {
				return TriggerRule{}, err
			}
			dates = append(dates, day)
		}
		rule := TriggerRule{Kind: TriggerSpecificDates, Dates: dates}
		if err := rule.Validate(); err != nil {
			return TriggerRule{}, err
		}
		return rule, nil
	case "every_n_days":
		if len(parts) != 4 || strings.ToLower(strings.TrimSpace(parts[2])) != "from" {
			return TriggerRule{}, malformed(text)
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return TriggerRule{}, malformed(text)
		}
		anchor, err := shared.ParseLocalDate(strings.TrimSpace(parts[3]))
		if err != nil {
			return TriggerRule{}, err
		}
		rule := TriggerRule{Kind: TriggerEveryNDays, EveryNDays: n, Anchor: anchor}
		if err := rule.Validate(); err != nil {
			return TriggerRule{}, err
		}
		return rule, nil
	}
	return TriggerRule{}, malformed(text)
}

func malformed(text string) error {
	return shared.Invalid("template.rule.parse", "rule", "unreadable trigger rule: "+text)
}
