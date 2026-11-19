package i18n

// Plural picks the form of a counted sentence: English uses the singular for
// one, Chinese has no plural inflection and always uses other. Both forms are
// required in both catalogs, so a translator can never leave one out.
func (l Lang) Plural(n int, one, other MessageID) MessageID {
	if l == En && n == 1 {
		return one
	}
	return other
}
