// Package notify shows the notification a finished pomodoro asks for. It is given a translate
// function, because the application layer sends message codes, not sentences.
package notify

import (
	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Notifier is the desktop notification adapter.
type Notifier struct {
	// Translate turns a message code into the sentence the user reads. A code
	// without a translation is shown as it is.
	Translate func(code string) string

	// send posts the notification. It is nil in the application, where the
	// platform is used, and replaced by a recorder in the tests: a test must not
	// put a toast on the screen of the machine that runs it.
	send func(app.Notification) error
}

// Notify shows one notification.
func (n Notifier) Notify(message shared.Notification) error {
	title, body := message.Title, message.Body
	if n.Translate != nil {
		title = n.Translate(title)
		body = n.Translate(body)
	}
	send := n.send
	if send == nil {
		send = app.SendNotification
	}
	return send(app.Notification{Title: title, Body: body})
}

// Capability reports whether this platform can show a notification at all.
func (n Notifier) Capability() shared.NotifyCapability {
	return shared.NotifyAvailable
}

var _ shared.Notifier = Notifier{}
