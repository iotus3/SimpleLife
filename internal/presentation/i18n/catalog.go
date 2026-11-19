package i18n

import "fmt"

// Catalog maps a message id to its sentence, with positional verbs (%[1]s,
// %[2]d) so a translation may reorder the arguments.
type Catalog map[MessageID]string

// T looks a sentence up. Lookup falls back to Chinese and then to the key
// itself: a key on screen is a loud, testable defect rather than a blank.
func T(l Lang, id MessageID, args ...any) string {
	text, ok := catalogOf(l)[id]
	if !ok || text == "" {
		if fallback, ok := catalogZh[id]; ok && fallback != "" {
			text = fallback
		} else {
			text = string(id)
		}
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// Has reports whether the language has a translation for id.
func Has(l Lang, id MessageID) bool {
	text, ok := catalogOf(l)[id]
	return ok && text != ""
}

func catalogOf(l Lang) Catalog {
	if l == En {
		return catalogEn
	}
	return catalogZh
}

// DisplayName resolves the name of a stored row: a built-in row keeps its key
// in builtinKey and is translated, while a name the user typed is shown as it
// is.
func DisplayName(l Lang, name, builtinKey string) string {
	if builtinKey == "" {
		return name
	}
	return T(l, MessageID(builtinKey))
}
