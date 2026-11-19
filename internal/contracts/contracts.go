// Package contracts holds the small data types that cross a context boundary; nothing here
// has behaviour.
package contracts

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// BookingDirection is which side of the ledger a booking hits.
type BookingDirection uint8

const (
	BookingIncome BookingDirection = iota + 1
	BookingExpense
)

// BookingRequest is one money movement produced by another context; the caller has already
// decided the amount.
type BookingRequest struct {
	Direction  BookingDirection
	Amount     shared.Money
	AccountID  shared.ID
	CategoryID shared.ID
	SessionID  shared.ID
	OccurredAt time.Time
	Note       string
}

// Booker writes a booking. Implemented by the ledger service; declared here so the
// focus service can depend on the capability without importing the ledger service.
type Booker interface {
	Book(req BookingRequest) error
}
