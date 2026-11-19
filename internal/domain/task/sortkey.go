package task

import (
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// The rank alphabet is 36 characters wide, lowercase only: SQLite compares TEXT with BINARY
// collation, and one case keeps the order identical everywhere.
const (
	rankAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	rankBase     = len(rankAlphabet)
)

// InitialSortKey is the key a brand-new task gets. "i" sits in the middle of the
// alphabet, leaving room to insert before and after it.
const InitialSortKey = "i"

func rankIndex(c byte) int {
	for i := 0; i < rankBase; i++ {
		if rankAlphabet[i] == c {
			return i
		}
	}
	return -1
}

func rankChar(i int) byte { return rankAlphabet[i] }

// RankBetween returns a key that sorts strictly between prev and next, so a reorder writes one
// row instead of renumbering the list. An empty prev inserts at the head, an empty next
// appends; both empty yields InitialSortKey. A key below the first one may not exist in
// string space, which is reported as task.rank.lowest so the caller can normalise and retry.
func RankBetween(prev, next string) (string, error) {
	if prev != "" && next != "" && prev >= next {
		return "", shared.Invalid("task.rank.order", "", "previous key must sort before next key")
	}
	if prev == "" && next == "" {
		return InitialSortKey, nil
	}
	out := make([]byte, 0, len(prev)+1)
	for i := 0; ; i++ {
		lo, hi := 0, rankBase
		if i < len(prev) {
			lo = rankIndex(prev[i])
			if lo < 0 {
				return "", shared.Invalid("task.rank.alphabet", "", "invalid character in sort key")
			}
		}
		if i < len(next) {
			hi = rankIndex(next[i])
			if hi < 0 {
				return "", shared.Invalid("task.rank.alphabet", "", "invalid character in sort key")
			}
		}
		if hi-lo > 1 {
			cand := string(append(out, rankChar((lo+hi)/2)))
			// Belt and braces: the arithmetic guarantees both comparisons, but a
			// wrong key silently reorders the user's list, so verify instead of
			// trusting the derivation.
			if prev != "" && cand <= prev {
				return "", shared.Invalid("task.rank.order", "", "cannot place a key after the previous one")
			}
			if next != "" && cand >= next {
				return "", shared.Invalid("task.rank.order", "", "cannot place a key before the next one")
			}
			return cand, nil
		}
		if hi == lo {
			// Equal digits: normally both bounds continue, so copy and go deeper.
			if i < len(prev) {
				out = append(out, rankChar(lo))
				continue
			}
			// prev is exhausted and next's next digit is the smallest one, so the
			// only key below next is the prefix itself — which must also sort
			// after prev.
			if len(out) > 0 && string(out) > prev {
				return string(out), nil
			}
			return "", shared.Invalid("task.rank.lowest", "", "no key sorts below the first item; normalise the list first")
		}
		// Adjacent digits: keep this position and look one character deeper. The
		// loop terminates because prev is finite — once it runs out, lo becomes 0
		// while hi stays rankBase.
		out = append(out, rankChar(lo))
	}
}

// NormalizeRanks produces n distinct, strictly increasing keys in list order. It
// is the repair path for data that drifted into duplicate keys (two imports, a
// manual database edit) — not something the normal insert path needs.
func NormalizeRanks(n int) []string {
	if n <= 0 {
		return nil
	}
	width := 1
	for pow := rankBase; pow <= n; pow *= rankBase {
		width++
	}
	total := 1
	for i := 0; i < width; i++ {
		total *= rankBase
	}
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		v := (i + 1) * total / (n + 1)
		if v < 1 {
			v = 1
		}
		if v > total-1 {
			v = total - 1
		}
		keys[i] = encodeRank(v, width)
	}
	return keys
}

func encodeRank(v, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = rankChar(v % rankBase)
		v /= rankBase
	}
	return string(buf)
}

// ValidRank reports whether s could have been produced by RankBetween.
func ValidRank(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if rankIndex(s[i]) < 0 {
			return false
		}
	}
	return true
}
