// Package i18n is the single source of user-visible text: the domain produces codes, this
// package produces sentences, keyed by a typed MessageID so a typo fails the build.
package i18n

// Lang is a supported interface language.
type Lang string

const (
	// ZhCN is the default language and the fallback of every lookup.
	ZhCN Lang = "zh-CN"
	// En is English.
	En Lang = "en"
)

// Preference values stored in settings. Auto follows the system.
const (
	PrefAuto = "auto"
	PrefZhCN = "zh-CN"
	PrefEn   = "en"
)

// ValidPreference reports whether pref is a language the settings may store.
func ValidPreference(pref string) bool {
	switch pref {
	case PrefAuto, PrefZhCN, PrefEn:
		return true
	}
	return false
}

// Langs lists the supported languages in menu order.
func Langs() []Lang { return []Lang{ZhCN, En} }

// Resolve decides the interface language: an explicit preference wins, otherwise the first
// system tag whose primary subtag is known, otherwise Chinese. Only the primary subtag is
// compared, so "zh-Hans-CN" and "zh-TW" both resolve.
func Resolve(pref string, systemTags []string) Lang {
	switch Lang(pref) {
	case ZhCN:
		return ZhCN
	case En:
		return En
	}
	for _, tag := range systemTags {
		primary := tag
		for i := 0; i < len(tag); i++ {
			if tag[i] == '-' || tag[i] == '_' {
				primary = tag[:i]
				break
			}
		}
		switch lower(primary) {
		case "zh":
			return ZhCN
		case "en":
			return En
		}
	}
	return ZhCN
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
