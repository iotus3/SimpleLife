package shared

import (
	"crypto/rand"
	"time"
)

// ID is a 26-character ULID-like identifier: a millisecond timestamp then random characters,
// Crockford base32, so sorting as strings sorts by creation time.
type ID string

const idLen = 26

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Valid reports whether id looks like an identifier this package produced.
func (id ID) Valid() bool {
	if len(id) != idLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if crockfordIndex(id[i]) < 0 {
			return false
		}
	}
	return true
}

func (id ID) String() string { return string(id) }

// IsZero reports an unset identifier.
func (id ID) IsZero() bool { return id == "" }

// IDGen mints identifiers. Injecting it keeps domain and application code
// deterministic under test.
type IDGen interface {
	New() ID
}

// NewID builds an identifier from a timestamp and 80 bits of randomness.
func NewID(now time.Time) (ID, error) {
	var buf [16]byte
	ms := uint64(now.UnixMilli())
	// 48 bits of timestamp, big endian, spread over the first 10 characters.
	buf[0] = byte(ms >> 40)
	buf[1] = byte(ms >> 32)
	buf[2] = byte(ms >> 24)
	buf[3] = byte(ms >> 16)
	buf[4] = byte(ms >> 8)
	buf[5] = byte(ms)
	if _, err := rand.Read(buf[6:]); err != nil {
		return "", err
	}
	// 128 bits -> 26 characters of 5 bits (130 bits, two spare).
	var out [idLen]byte
	var acc uint32
	var bits uint
	pos := 0
	for _, b := range buf {
		acc = acc<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[pos] = crockford[(acc>>bits)&0x1f]
			pos++
		}
	}
	if bits > 0 {
		out[pos] = crockford[(acc<<(5-bits))&0x1f]
		pos++
	}
	for ; pos < idLen; pos++ {
		out[pos] = crockford[0]
	}
	return ID(out[:]), nil
}

func crockfordIndex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'H':
		return int(c-'A') + 10
	case c >= 'J' && c <= 'K':
		return int(c-'J') + 18
	case c >= 'M' && c <= 'N':
		return int(c-'M') + 20
	case c >= 'P' && c <= 'T':
		return int(c-'P') + 22
	case c >= 'V' && c <= 'Z':
		return int(c-'V') + 27
	}
	return -1
}
