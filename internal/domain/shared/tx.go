package shared

// TxRunner runs a group of repository writes as one unit; the mechanism belongs to the
// infrastructure.
type TxRunner interface {
	WithinTx(fn func() error) error
}

// DirectTxRunner runs fn with no transaction. It is the zero-cost default for
// tests and for adapters whose writes are single-row.
type DirectTxRunner struct{}

// WithinTx implements TxRunner.
func (DirectTxRunner) WithinTx(fn func() error) error { return fn() }
