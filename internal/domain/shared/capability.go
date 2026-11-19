package shared

// Notification is a short system message. Tag lets the adapter collapse repeats
// of the same reminder instead of stacking them.
type Notification struct {
	Title string
	Body  string
	Tag   string
}

// NotifyCapability says what the running build can promise about notifications.
type NotifyCapability uint8

const (
	// NotifyAvailable means a notification will be shown without further steps.
	NotifyAvailable NotifyCapability = iota + 1
	// NotifyNeedsPermission means the user must grant permission first (browser).
	NotifyNeedsPermission
	// NotifyUnsupported means there is nowhere to show it; the caller falls back
	// to an in-window toast. It is never treated as success.
	NotifyUnsupported
)

// Notifier delivers system notifications. Every error it returns is advisory:
// a failed reminder must never fail the use case that triggered it.
type Notifier interface {
	Notify(n Notification) error
	Capability() NotifyCapability
}

// NoopNotifier drops notifications; the zero-value choice for tests and for
// builds without a notification centre.
type NoopNotifier struct{}

// Notify implements Notifier.
func (NoopNotifier) Notify(Notification) error { return nil }

// Capability implements Notifier.
func (NoopNotifier) Capability() NotifyCapability { return NotifyUnsupported }

// SleepGuard keeps the machine awake while a focus session runs. The returned
// release function must be idempotent-safe: calling it twice is allowed.
type SleepGuard interface {
	Hold(reason string) (release func(), err error)
}

// NoopSleepGuard holds nothing. It is what the browser build uses, and it is
// valid to return ErrUnsupportedPlatform instead when the UI wants to say so.
type NoopSleepGuard struct{}

// Hold implements SleepGuard.
func (NoopSleepGuard) Hold(string) (func(), error) { return func() {}, nil }
